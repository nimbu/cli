package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func writePageFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "page.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write page file: %v", err)
	}
	return path
}

func TestPagesUpdateDefaultOmitsReplace(t *testing.T) {
	var gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/pages/about" {
			gotRawQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"id":"p1","items":{"intro":{"type":"string"}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	file := writePageFile(t, `{"id":"old","items":{"intro":{"type":"string"}}}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages update: %v", err)
	}
	if strings.Contains(gotRawQuery, "replace=1") {
		t.Fatalf("default update should not send replace=1, got %q", gotRawQuery)
	}
}

func TestPagesUpdateFromFilePreservesParentFullpath(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/pages/about/team" {
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"p1","parent":"about","items":{}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	file := writePageFile(t, `{"parent":"about","parent_path":"old","items":{}}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about/team", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages update: %v", err)
	}
	if gotBody["parent"] != "about" {
		t.Fatalf("expected parent fullpath preserved, got %#v", gotBody["parent"])
	}
	if _, ok := gotBody["parent_path"]; ok {
		t.Fatalf("parent_path should remain stripped as read-only, got %#v", gotBody)
	}
}

func TestPagesUpdateInlineDoesNotEchoFetchedItems(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(`{"id":"p1","fullpath":"about","title":"Old","items":{"hero":{"type":"file","file":{"url":"https://cdn.example.test/hero.jpg","filename":"hero.jpg"}}}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/about":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"p1","fullpath":"about","title":"New","items":{"hero":{"type":"file","file":{"url":"https://cdn.example.test/hero.jpg"}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", Assignments: []string{"title=New"}}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages update: %v", err)
	}
	if gotBody["title"] != "New" {
		t.Fatalf("expected title update, got %#v", gotBody["title"])
	}
	if _, ok := gotBody["items"]; ok {
		t.Fatalf("fetched items (and their read-only file urls) must not be sent in an inline merge, got %#v", gotBody["items"])
	}
}

func TestPagesUpdateInlineTranslationsSendsOnlySuppliedBlock(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			// en.seo_title is a locale fallback of the nl value; the server
			// does not mark it, so it must never be written back.
			_, _ = w.Write([]byte(`{
				"id":"p1",
				"fullpath":"about",
				"title":"About",
				"translations":{
					"en":{"seo_title":"Oude titel"},
					"nl":{"seo_title":"Oude titel","seo_description":"Blijft behouden"}
				}
			}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/about":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"p1","fullpath":"about"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{
		Page:        "about",
		Assignments: []string{`translations:={"nl":{"seo_title":"Nieuwe titel"},"fr":{"seo_title":"Titre"}}`},
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages update: %v", err)
	}

	translations := gotBody["translations"].(map[string]any)
	if _, ok := translations["en"]; ok {
		t.Fatalf("fetched (fallback) English translation must not be echoed back: %#v", translations)
	}
	nl := translations["nl"].(map[string]any)
	if nl["seo_title"] != "Nieuwe titel" || len(nl) != 1 {
		t.Fatalf("Dutch block should be sent as supplied: %#v", nl)
	}
	if fr := translations["fr"].(map[string]any); fr["seo_title"] != "Titre" {
		t.Fatalf("French translation missing: %#v", fr)
	}
	for _, key := range []string{"title", "fullpath", "id"} {
		if _, ok := gotBody[key]; ok {
			t.Fatalf("fetched %q must not be written back: %#v", key, gotBody)
		}
	}
}

func TestPagesUpdateReplaceRejectsInlineAssignments(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "http://127.0.0.1:1", output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", Replace: true, Assignments: []string{"title=New"}}

	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatalf("expected --replace with inline assignments to error")
	}
	if !strings.Contains(err.Error(), "--replace requires --file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPagesUpdateReplaceRequiresFile(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "http://127.0.0.1:1", output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", Replace: true}

	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatalf("expected bare --replace to error")
	}
	if !strings.Contains(err.Error(), "--replace requires --file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPagesUpdateReplaceAddsParam(t *testing.T) {
	var patchRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(`{"id":"p1","items":{"blocks":{"type":"canvas","repeatables":[{"slug":"a"}]}}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/about":
			patchRawQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"id":"p1","items":{"blocks":{"type":"canvas","repeatables":[{"slug":"a"},{"slug":"b"}]}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	file := writePageFile(t, `{"items":{"blocks":{"type":"canvas","repeatables":[{"slug":"a"},{"slug":"b"}]}}}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", File: file, Replace: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages update --replace: %v", err)
	}
	if !strings.Contains(patchRawQuery, "replace=1") {
		t.Fatalf("expected replace=1 on --replace, got %q", patchRawQuery)
	}
}

func TestPagesUpdateReplaceGuardsCanvasWipe(t *testing.T) {
	patchCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(`{"id":"p1","items":{"blocks":{"type":"canvas","repeatables":[{"slug":"a"},{"slug":"b"}]}}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/about":
			patchCalled = true
			_, _ = w.Write([]byte(`{"id":"p1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	file := writePageFile(t, `{"items":{"blocks":{"type":"canvas","repeatables":[]}}}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", File: file, Replace: true}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "would be wiped from 2->0") {
		t.Fatalf("expected canvas-wipe guard error, got %v", err)
	}
	if patchCalled {
		t.Fatalf("guard should abort before PATCH")
	}
}

func TestPagesUpdateAllowEmptyCanvasOverridesGuard(t *testing.T) {
	patchCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(`{"id":"p1","items":{"blocks":{"type":"canvas","repeatables":[{"slug":"a"}]}}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/about":
			patchCalled = true
			_, _ = w.Write([]byte(`{"id":"p1","items":{"blocks":{"type":"canvas","repeatables":[]}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	file := writePageFile(t, `{"items":{"blocks":{"type":"canvas","repeatables":[]}}}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", File: file, Replace: true, AllowEmptyCanvas: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages update --allow-empty-canvas: %v", err)
	}
	if !patchCalled {
		t.Fatalf("expected PATCH to proceed with --allow-empty-canvas")
	}
}

func TestPagesUpdateMergeGuardsExplicitCanvasWipe(t *testing.T) {
	patchCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(`{"id":"p1","items":{"blocks":{"type":"canvas","repeatables":[{"slug":"a"},{"slug":"b"}]}}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/about":
			patchCalled = true
			_, _ = w.Write([]byte(`{"id":"p1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	file := writePageFile(t, `{"items":{"blocks":{"type":"canvas","repeatables":[]}}}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", File: file}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "canvas 'blocks' would be wiped from 2->0") {
		t.Fatalf("expected explicit canvas-wipe guard error, got %v", err)
	}
	if patchCalled {
		t.Fatalf("guard should abort before PATCH")
	}
}

func TestPagesUpdateReplaceGuardsNestedCanvasWipe(t *testing.T) {
	patchCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(`{"id":"p1","items":{"blocks":{"type":"canvas","repeatables":[{"slug":"section","items":{"gallery":{"type":"canvas","repeatables":[{"slug":"image"},{"slug":"image"}]}}}]}}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/about":
			patchCalled = true
			_, _ = w.Write([]byte(`{"id":"p1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	file := writePageFile(t, `{"items":{"blocks":{"type":"canvas","repeatables":[{"slug":"section","items":{"gallery":{"type":"canvas","repeatables":[]}}}]}}}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", File: file, Replace: true}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "canvas 'blocks.gallery' would be wiped from 2->0") {
		t.Fatalf("expected nested canvas-wipe guard error, got %v", err)
	}
	if patchCalled {
		t.Fatalf("guard should abort before PATCH")
	}
}

func TestPagesUpdateReplaceGuardsNestedCanvasPartialWipe(t *testing.T) {
	patchCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(`{"id":"p1","items":{"blocks":{"type":"canvas","repeatables":[{"slug":"section","items":{"gallery":{"type":"canvas","repeatables":[{"slug":"image"},{"slug":"image"}]}}},{"slug":"section","items":{"gallery":{"type":"canvas","repeatables":[{"slug":"image"}]}}}]}}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/about":
			patchCalled = true
			_, _ = w.Write([]byte(`{"id":"p1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	file := writePageFile(t, `{"items":{"blocks":{"type":"canvas","repeatables":[{"slug":"section","items":{"gallery":{"type":"canvas","repeatables":[]}}},{"slug":"section","items":{"gallery":{"type":"canvas","repeatables":[{"slug":"image"}]}}}]}}}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", File: file, Replace: true}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "canvas 'blocks[0].gallery' would be wiped from 2->0") {
		t.Fatalf("expected nested partial canvas-wipe guard error, got %v", err)
	}
	if patchCalled {
		t.Fatalf("guard should abort before PATCH")
	}
}

func TestPagesUpdateAllowEmptyFileOverridesGuard(t *testing.T) {
	patchCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/pages/about" {
			patchCalled = true
			_, _ = w.Write([]byte(`{"id":"p1","items":{"hero":{"type":"file","file":{}}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	file := writePageFile(t, `{"items":{"hero":{"type":"file","file":{"filename":"hero.jpg"}}}}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", File: file, AllowEmptyFile: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages update --allow-empty-file: %v", err)
	}
	if !patchCalled {
		t.Fatalf("expected PATCH to proceed with --allow-empty-file")
	}
}

func TestPagesUpdateDryRunPrintsBodyWithoutPatch(t *testing.T) {
	var patched bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(`{"id":"p1","fullpath":"about","title":"Old","template":"page","items":{}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/about":
			patched = true
			_, _ = w.Write([]byte(`{"id":"p1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, errOut := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{Page: "about", Assignments: []string{"title=New"}, DryRun: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", Readonly: true}); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if patched {
		t.Fatal("dry-run must not PATCH")
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not a single JSON object: %v\n%s", err, out)
	}
	if payload["dry_run"] != true {
		t.Fatalf("stdout = %s", out)
	}
	body, _ := payload["body"].(map[string]any)
	if body["title"] != "New" {
		t.Fatalf("body = %#v", payload["body"])
	}
	if !strings.Contains(errOut.String(), "dry-run: no request sent (the API has no validation endpoint yet)") {
		t.Fatalf("stderr = %q", errOut.String())
	}
	if strings.Contains(out.String(), "dry-run: no request sent") {
		t.Fatalf("stderr note leaked onto stdout: %s", out)
	}
}

func TestPagesUpdateDryRunFlagParses(t *testing.T) {
	parser, cli, err := newParser()
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}
	if _, err := parser.Parse([]string{"pages", "update", "--page=about", "--dry-run", "title=New"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cli.Pages.Update.DryRun {
		t.Fatal("expected --dry-run")
	}
}

func TestPagesUpdatePrintsAppliedCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/pages/about" {
			_, _ = w.Write([]byte(`{"id":"p1","items":{"intro":{"type":"string"},"hero":{"file":{"url":"https://x/y.jpg"}}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	file := writePageFile(t, `{"items":{"intro":{"type":"string"}}}`)
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesUpdateCmd{Page: "about", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages update: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Updated page p1 (2 editables, 1 attachment)") {
		t.Fatalf("unexpected human output: %q", got)
	}
}

func TestPagesUpdateInlineDoesNotEchoFetchedTranslations(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/nl-slug":
			// A nl-default site with no English translation: the API fills
			// every locale value with the nl fallback and nothing marks it.
			_, _ = w.Write([]byte(`{
				"id":"p1",
				"slug":"nl-slug",
				"fullpath":"nl-slug",
				"public_url":"https://demo.test/nl-slug",
				"depth":0,
				"title":"Nederlandse titel",
				"template":"page",
				"translations":{
					"nl":{"slug":"nl-slug","title":"Nederlandse titel"},
					"en":{"slug":"nl-slug","title":"Nederlandse titel"}
				}
			}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/nl-slug":
			if !strings.Contains(r.URL.RawQuery, "content_locale=en") {
				t.Fatalf("missing content_locale: %q", r.URL.RawQuery)
			}
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"p1","fullpath":"nl-slug","title":"English"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesUpdateCmd{QueryFlags: QueryFlags{Locale: "en"}, Page: "nl-slug", Assignments: []string{"title=English"}}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages update: %v", err)
	}
	if gotBody["title"] != "English" {
		t.Fatalf("expected title update, got %#v", gotBody)
	}
	for _, key := range []string{"translations", "fullpath", "public_url", "depth", "slug"} {
		if _, ok := gotBody[key]; ok {
			t.Fatalf("fetched %q must not be written back, got %#v", key, gotBody)
		}
	}
}
