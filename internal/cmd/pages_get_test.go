package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func outlinePageJSON() string {
	return `{
		"id":"p1",
		"fullpath":"about",
		"title":"Over ons",
		"slug":"about",
		"published":true,
		"template":"page",
		"created_at":"2026-01-01T00:00:00.000Z",
		"updated_at":"2026-01-02T00:00:00.000Z",
		"translations":{
			"nl":{"title":"Over ons"},
			"en":{"title":"About us"}
		},
		"items":{
			"Theme":{"type":"select","slug":"Theme","content":"Light"},
			"Empty":{"type":"text","slug":"Empty","content":""},
			"Blokken":{
				"type":"canvas",
				"slug":"Blokken",
				"repeatables":[
					{
						"id":"6aa2bc6155bb5c0aac12218e",
						"slug":"hero_stage",
						"position":1,
						"created_at":"2026-01-01T00:00:00.000Z",
						"updated_at":"2026-01-02T00:00:00.000Z",
						"items":{
							"Title":{
								"type":"text",
								"slug":"Title",
								"created_at":"2026-01-01T00:00:00.000Z",
								"updated_at":"2026-01-02T00:00:00.000Z",
								"content":"<p>Een webapplicatie   laten maken</p>",
								"translations":{
									"nl":{"content":"<p>Een webapplicatie   laten maken</p>","updated_at":"2026-01-02T00:00:00.000Z"},
									"en":{"content":"<p>Build a web application</p>"}
								}
							},
							"Photos":{
								"type":"canvas",
								"repeatables":[
									{
										"id":"6aa2bc6155bb5c0aac122190",
										"slug":"photo",
										"position":1,
										"items":{
											"Image":{
												"type":"file",
												"slug":"Image",
												"file":{
													"url":"https://cdn.example.test/rreuse.png",
													"filename":"rreuse.png",
													"width":100,
													"height":50,
													"content_type":"image/png",
													"size":1234
												}
											}
										}
									}
								]
							}
						}
					}
				]
			}
		}
	}`
}

func outlinePageServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/pages/about" {
			_, _ = w.Write([]byte(outlinePageJSON()))
			return
		}
		http.NotFound(w, r)
	}))
}

// Regression: --locale used to be ignored for --json, which returned the
// default-locale content while the text output was localized.
func TestPagesGetJSONHonoursLocale(t *testing.T) {
	srv := outlinePageServer(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesGetCmd{Page: "about"}
	cmd.Locale = "en"
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages get: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got["title"] != "About us" {
		t.Fatalf("expected English title, got %#v", got["title"])
	}
	if _, ok := got["translations"].(map[string]any); !ok {
		t.Fatalf("translations map should stay present, got %#v", got["translations"])
	}
	items := got["items"].(map[string]any)
	reps := items["Blokken"].(map[string]any)["repeatables"].([]any)
	hero := reps[0].(map[string]any)["items"].(map[string]any)
	title := hero["Title"].(map[string]any)
	if title["content"] != "<p>Build a web application</p>" {
		t.Fatalf("expected English editable content, got %#v", title["content"])
	}
}

func TestPagesGetJSONWithoutLocaleKeepsDefault(t *testing.T) {
	srv := outlinePageServer(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesGetCmd{Page: "about"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages get: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got["title"] != "Over ons" {
		t.Fatalf("expected default title, got %#v", got["title"])
	}
}

func TestPagesGetOutlineText(t *testing.T) {
	srv := outlinePageServer(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesGetCmd{Page: "about", Outline: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages get: %v", err)
	}

	want := strings.Join([]string{
		"title (page): Over ons",
		"slug (page): about",
		"published (page): true",
		"template (page): page",
		"Blokken[0] hero_stage 6aa2bc6155bb5c0aac12218e",
		"  Blokken[0].Photos (canvas)",
		"    Blokken[0].Photos[0] photo 6aa2bc6155bb5c0aac122190",
		"      Blokken[0].Photos[0].Image (file): rreuse.png",
		"  Blokken[0].Title (text): <p>Een webapplicatie laten maken</p>",
		"Empty (text): (empty)",
		"Theme (select): Light",
		"",
	}, "\n")
	if out.String() != want {
		t.Fatalf("outline =\n%s\nwant\n%s", out.String(), want)
	}
}

func TestPagesGetOutlineTextHonoursLocale(t *testing.T) {
	srv := outlinePageServer(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesGetCmd{Page: "about", Outline: true}
	cmd.Locale = "en"
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages get: %v", err)
	}
	if !strings.Contains(out.String(), "Title (text): <p>Build a web application</p>") {
		t.Fatalf("expected English content in outline, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "title (page): About us") {
		t.Fatalf("expected English page title in outline, got:\n%s", out.String())
	}
	// Falls back to the default locale when a translation is missing.
	if !strings.Contains(out.String(), "Theme (select): Light") {
		t.Fatalf("expected default fallback, got:\n%s", out.String())
	}
}

func TestPagesGetOutlineJSON(t *testing.T) {
	srv := outlinePageServer(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesGetCmd{Page: "about", Outline: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages get: %v", err)
	}

	var entries []map[string]any
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	byPath := map[string]map[string]any{}
	for _, entry := range entries {
		byPath[entry["path"].(string)] = entry
	}

	title := byPath["Blokken[0].Title"]
	if title == nil {
		t.Fatalf("missing Blokken[0].Title entry: %#v", byPath)
	}
	if title["raw_path"] != "/items/Blokken/repeatables/6aa2bc6155bb5c0aac12218e/items/Title" {
		t.Fatalf("raw_path = %#v", title["raw_path"])
	}
	if title["type"] != "text" || title["slug"] != "Title" {
		t.Fatalf("type/slug = %#v", title)
	}
	if title["content"] != "<p>Een webapplicatie   laten maken</p>" {
		t.Fatalf("content should be untruncated and unmodified, got %#v", title["content"])
	}

	rep := byPath["Blokken[0]"]
	if rep == nil || rep["id"] != "6aa2bc6155bb5c0aac12218e" || rep["type"] != "repeatable" || rep["slug"] != "hero_stage" {
		t.Fatalf("repeatable entry = %#v", rep)
	}
	if rep["raw_path"] != "/items/Blokken/repeatables/6aa2bc6155bb5c0aac12218e" {
		t.Fatalf("repeatable raw_path = %#v", rep["raw_path"])
	}

	nested := byPath["Blokken[0].Photos[0].Image"]
	if nested == nil || nested["type"] != "file" {
		t.Fatalf("nested file entry = %#v", nested)
	}
	if entries[0]["path"] != "title" || entries[0]["type"] != "page" {
		t.Fatalf("page fields should come first, got %#v", entries[0])
	}
}

func TestPagesGetOutlineRejectsShape(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "http://127.0.0.1:1", output.Mode{JSON: true})
	cmd := &PagesGetCmd{Page: "about", Outline: true, Shape: true}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual exclusion error, got %v", err)
	}
}

func TestPagesGetCompactJSON(t *testing.T) {
	srv := outlinePageServer(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesGetCmd{Page: "about", Compact: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages get: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	items := got["items"].(map[string]any)
	theme := items["Theme"].(map[string]any)
	for _, key := range []string{"slug", "type", "created_at", "updated_at"} {
		if _, ok := theme[key]; ok {
			t.Fatalf("editable should drop %q, got %#v", key, theme)
		}
	}
	if theme["content"] != "Light" {
		t.Fatalf("content must survive, got %#v", theme)
	}

	blokken := items["Blokken"].(map[string]any)
	rep := blokken["repeatables"].([]any)[0].(map[string]any)
	if rep["id"] != "6aa2bc6155bb5c0aac12218e" || rep["position"] == nil || rep["slug"] != "hero_stage" {
		t.Fatalf("ids, positions and slugs must be kept, got %#v", rep)
	}
	for _, key := range []string{"created_at", "updated_at"} {
		if _, ok := rep[key]; ok {
			t.Fatalf("repeatable should drop %q, got %#v", key, rep)
		}
	}
	hero := rep["items"].(map[string]any)
	title := hero["Title"].(map[string]any)
	translations, ok := title["translations"].(map[string]any)
	if !ok {
		t.Fatalf("translations should keep the non-default locale, got %#v", title)
	}
	if _, ok := translations["nl"]; ok {
		t.Fatalf("redundant default-locale translation should be dropped, got %#v", translations)
	}
	if _, ok := translations["en"]; !ok {
		t.Fatalf("other locales must survive, got %#v", translations)
	}

	photo := hero["Photos"].(map[string]any)["repeatables"].([]any)[0].(map[string]any)
	file := photo["items"].(map[string]any)["Image"].(map[string]any)["file"].(map[string]any)
	if len(file) != 4 || file["filename"] != "rreuse.png" || file["url"] == nil {
		t.Fatalf("file should reduce to url/filename/width/height, got %#v", file)
	}
	if _, ok := got["translations"]; !ok {
		t.Fatalf("page translations with a real alternative locale must survive: %#v", got["translations"])
	}
}

func TestPagesGetFieldsProjection(t *testing.T) {
	srv := outlinePageServer(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesGetCmd{Page: "about"}
	cmd.Fields = "id, title,items"
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages get: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected only the requested fields, got %#v", got)
	}
	for _, key := range []string{"id", "title", "items"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing field %q: %#v", key, got)
		}
	}
	if _, ok := got["items"].(map[string]any)["Blokken"]; !ok {
		t.Fatalf("items must stay whole, got %#v", got["items"])
	}
}

func TestPagesGetFieldsUnknownErrors(t *testing.T) {
	srv := outlinePageServer(t)
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesGetCmd{Page: "about"}
	cmd.Fields = "title,nope"
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

// Regression: --compact deleted the attachment_path that --download-assets had
// just written, so the compacted document could not be written back.
func TestPagesGetCompactKeepsDownloadedAttachmentPath(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pages/about":
			page := `{
				"id":"p1",
				"title":"About",
				"og_image":{"url":"` + srv.URL + `/assets/og.png","filename":"og.png","size":2},
				"items":{
					"Hero":{"type":"file","slug":"Hero","file":{"url":"` + srv.URL + `/assets/hero.png","filename":"hero.png","width":1,"height":2,"size":2}}
				}
			}`
			_, _ = w.Write([]byte(page))
		case "/assets/hero.png", "/assets/og.png":
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesGetCmd{Page: "about", Compact: true, DownloadAssets: dir}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages get: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	file := got["items"].(map[string]any)["Hero"].(map[string]any)["file"].(map[string]any)
	path, _ := file["attachment_path"].(string)
	if path == "" {
		t.Fatalf("compact dropped attachment_path written by --download-assets: %#v", file)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("attachment_path %q does not exist: %v", path, err)
	}
	if _, ok := file["size"]; ok {
		t.Fatalf("compaction should still drop noise keys: %#v", file)
	}
}
