package cmd

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestPagesInsertPositionAndAfter(t *testing.T) {
	t.Run("position 0 is after null", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		zero := 0
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesInsertCmd{Page: "about", Path: "Blokken", Slug: "hero_stage", Position: &zero}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
		if op["op"] != "insert" || op["path"] != "/items/Blokken/repeatables" {
			t.Fatalf("op = %#v", op)
		}
		if op["after"] != nil {
			t.Fatalf("after = %#v, want null", op["after"])
		}
	})

	t.Run("after index uses sibling id", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesInsertCmd{Page: "about", Path: "Blokken", Slug: "proof_strip", After: "0"}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
		if op["after"] != surgicalBlockID {
			t.Fatalf("after = %#v", op["after"])
		}
	})
}

func TestPagesInsertFileEditableUsesTwoBatches(t *testing.T) {
	var etags []string
	var firstItems map[string]any
	srvState := &surgicalServer{}
	srvState.batchFn = func(w http.ResponseWriter, r *http.Request, n int) {
		etags = append(etags, r.Header.Get("If-Match"))
		if n == 1 {
			firstItems = insertOpItems(t, srvState.lastBody)
			_, _ = w.Write([]byte(`{
				"results":[{"index":0,"status":"ok","path":"/items/Blokken/repeatables","id":"newblock000000000000001"}],
				"etag":"etag-from-insert",
				"updated_at":"2026-09-10T16:00:00.000Z",
				"page":` + surgicalPageJSON() + `
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"results":[{"index":0,"status":"ok","path":"/items/Blokken/repeatables/newblock000000000000001/items/Image"}],
			"etag":"etag-from-fileset",
			"updated_at":"2026-09-10T16:00:01.000Z",
			"page":` + surgicalPageJSON() + `
		}`))
	}
	srv := srvState.start(t)
	defer srv.Close()

	file := writePageFile(t, `{"Title":"Hi","Image":{"attachment_url":"https://cdn.example.test/a.jpg"}}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesInsertCmd{Page: "about", Path: "Blokken", Slug: "hero_stage", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if srvState.posts != 2 {
		t.Fatalf("posts = %d, want 2", srvState.posts)
	}
	if etags[1] != `"etag-from-insert"` {
		t.Fatalf("second If-Match = %s", etags[1])
	}
	if firstItems["Title"] != "Hi" || firstItems["Image"] != nil || firstItems["Enabled"] != nil || firstItems["Ref"] != nil {
		t.Fatalf("seeded insert items = %#v", firstItems)
	}
	second := srvState.lastBody["operations"].([]any)[0].(map[string]any)
	if second["op"] != "set" || !strings.Contains(second["path"].(string), "newblock000000000000001/items/Image") {
		t.Fatalf("second op = %#v", second)
	}
}

func TestPagesInsertBadSlugListsAllowed(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesInsertCmd{Page: "about", Path: "Blokken", Slug: "nope"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected slug error")
	}
	if !strings.Contains(err.Error(), "hero_stage") || !strings.Contains(err.Error(), "proof_strip") {
		t.Fatalf("error = %v", err)
	}
}

func TestPagesInsertBadSlugListsAllowedFromLiveSchema(t *testing.T) {
	live, err := os.ReadFile(filepath.Join("..", "pagepath", "testdata", "schema_live.json"))
	if err != nil {
		t.Fatal(err)
	}
	srvState := &surgicalServer{schemaJSON: string(live)}
	srv := srvState.start(t)
	defer srv.Close()
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesInsertCmd{Page: "about", Path: "Blokken", Slug: "nope"}
	err = cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected slug error")
	}
	got := err.Error()
	if !strings.Contains(got, "hero_stage") || !strings.Contains(got, "allowed:") {
		t.Fatalf("error = %v", err)
	}
}

func TestPagesInsertSeedsSchemaFields(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()
	file := writePageFile(t, `{"Title":"Hello"}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesInsertCmd{Page: "about", Path: "Blokken", Slug: "hero_stage", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	items := insertOpItems(t, srvState.lastBody)
	if items["Title"] != "Hello" {
		t.Fatalf("Title = %#v", items["Title"])
	}
	for _, key := range []string{"Enabled", "Image", "Ref"} {
		if v, ok := items[key]; !ok || v != nil {
			t.Fatalf("%s = %#v, want seeded null", key, items[key])
		}
	}
	if len(items) != 4 {
		t.Fatalf("items = %#v", items)
	}
}

func TestPagesInsertUnknownEditableListsBlockFields(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()
	file := writePageFile(t, `{"Title":"Hi","Nope":true}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesInsertCmd{Page: "about", Path: "Blokken", Slug: "hero_stage", File: file}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected unknown editable error")
	}
	got := err.Error()
	if !strings.Contains(got, `editable "Nope" is not part of block hero_stage`) {
		t.Fatalf("error = %v", err)
	}
	for _, part := range []string{"Title (text)", "Enabled (switch)", "Image (file)", "Ref (reference)"} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %q in %v", part, err)
		}
	}
}

func TestPagesInsertNestedSeedsFromLiveSchema(t *testing.T) {
	live, err := os.ReadFile(filepath.Join("..", "pagepath", "testdata", "schema_live.json"))
	if err != nil {
		t.Fatal(err)
	}
	srvState := &surgicalServer{
		schemaJSON: string(live),
		pageJSON:   surgicalPageWithNestedPhotos(),
	}
	srv := srvState.start(t)
	defer srv.Close()
	file := writePageFile(t, `{"Alt":"caption"}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesInsertCmd{Page: "about", Path: "Blokken[0].Photos", Slug: "photo", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
	if op["path"] != "/items/Blokken/repeatables/"+surgicalBlockID+"/items/Photos/repeatables" {
		t.Fatalf("path = %#v", op["path"])
	}
	items := insertOpItems(t, srvState.lastBody)
	if items["Alt"] != "caption" {
		t.Fatalf("Alt = %#v", items["Alt"])
	}
	for _, key := range []string{"Image", "Position"} {
		if v, ok := items[key]; !ok || v != nil {
			t.Fatalf("%s = %#v, want seeded null", key, items[key])
		}
	}
	if len(items) != 3 {
		t.Fatalf("items = %#v", items)
	}
}

func TestPagesInsertDryRunShowsSeededOperation(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()
	file := writePageFile(t, `{"Title":"Hello"}`)
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesInsertCmd{Page: "about", Path: "Blokken", Slug: "hero_stage", File: file, DryRun: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if srvState.posts != 0 {
		t.Fatalf("dry-run posted %d times", srvState.posts)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	ops := body["operations"].([]any)
	if len(ops) != 1 {
		t.Fatalf("operations = %#v", body)
	}
	items := ops[0].(map[string]any)["value"].(map[string]any)["items"].(map[string]any)
	if items["Title"] != "Hello" {
		t.Fatalf("Title = %#v", items["Title"])
	}
	for _, key := range []string{"Enabled", "Image", "Ref"} {
		if v, ok := items[key]; !ok || v != nil {
			t.Fatalf("%s = %#v, want seeded null", key, items[key])
		}
	}
}

func TestPagesInsertWithoutTemplateSendsUserItems(t *testing.T) {
	srvState := &surgicalServer{schemaJSON: `{"template":{},"available_blocks":{}}`}
	srv := srvState.start(t)
	defer srv.Close()
	file := writePageFile(t, `{"Title":"Only"}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesInsertCmd{Page: "about", Path: "Blokken", Slug: "custom", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	items := insertOpItems(t, srvState.lastBody)
	if len(items) != 1 || items["Title"] != "Only" {
		t.Fatalf("items = %#v", items)
	}
}

func insertOpItems(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	op := body["operations"].([]any)[0].(map[string]any)
	value := op["value"].(map[string]any)
	items, ok := value["items"].(map[string]any)
	if !ok {
		t.Fatalf("items = %#v", value["items"])
	}
	return items
}

func surgicalPageWithNestedPhotos() string {
	return strings.Replace(surgicalPageJSON(),
		`"Ref":{"type":"reference"}`,
		`"Ref":{"type":"reference"},"Photos":{"type":"canvas","repeatables":[]}`,
		1)
}
