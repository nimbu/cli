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

func TestPagesUpdateInlineDropsReadOnlyFileURL(t *testing.T) {
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
	items := gotBody["items"].(map[string]any)
	hero := items["hero"].(map[string]any)
	if _, ok := hero["file"]; ok {
		t.Fatalf("read-only file url should not be sent in merge payload, got %#v", hero["file"])
	}
}

func TestPagesUpdateInlineDropsEmptyFileMap(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(`{"id":"p1","fullpath":"about","title":"Old","items":{"hero":{"type":"file","file":{"filename":"hero.jpg"}}}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/about":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"p1","fullpath":"about","title":"New","items":{"hero":{"type":"file"}}}`))
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
	items := gotBody["items"].(map[string]any)
	hero := items["hero"].(map[string]any)
	if _, ok := hero["file"]; ok {
		t.Fatalf("empty file map should not be sent in inline merge payload, got %#v", hero["file"])
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
