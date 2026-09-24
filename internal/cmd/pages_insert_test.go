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

func TestPagesInsertKeepsFileEditablesInsideInsert(t *testing.T) {
	newAsset := func(t *testing.T, hits *int) *httptest.Server {
		t.Helper()
		asset := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hits != nil {
				*hits++
			}
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg-bytes"))
		}))
		t.Cleanup(asset.Close)
		return asset
	}
	runInsert := func(t *testing.T, srvState *surgicalServer, items string, dryRun bool) string {
		t.Helper()
		srv := srvState.start(t)
		defer srv.Close()
		ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: dryRun})
		cmd := &PagesInsertCmd{Page: "about", Path: "Blokken", Slug: "hero_stage", File: writePageFile(t, items), DryRun: dryRun}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		return out.String()
	}
	singleInsertItems := func(t *testing.T, srvState *surgicalServer) map[string]any {
		t.Helper()
		if srvState.posts != 1 {
			t.Fatalf("posts = %d, want one batch with the file inside the insert", srvState.posts)
		}
		ops := srvState.lastBody["operations"].([]any)
		if len(ops) != 1 || ops[0].(map[string]any)["op"] != "insert" {
			t.Fatalf("operations = %#v", ops)
		}
		return insertOpItems(t, srvState.lastBody)
	}

	t.Run("foreign url is inlined as data", func(t *testing.T) {
		asset := newAsset(t, nil)
		srvState := &surgicalServer{}
		runInsert(t, srvState, `{"Title":"Hi","Image":{"attachment_url":"`+asset.URL+`/a.jpg"}}`, false)
		items := singleInsertItems(t, srvState)
		image, _ := items["Image"].(map[string]any)
		if image["data"] == nil || image["filename"] != "a.jpg" || image["content_type"] != "image/jpeg" {
			t.Fatalf("Image = %#v, want {data,filename,content_type}", items["Image"])
		}
		if items["Title"] != "Hi" || items["Enabled"] != nil || items["Ref"] != nil {
			t.Fatalf("seeded insert items = %#v", items)
		}
	})

	t.Run("same-site upload url becomes a FileRef", func(t *testing.T) {
		srvState := &surgicalServer{extraFn: serveSameSiteUpload}
		runInsert(t, srvState, `{"Image":{"attachment_url":"`+fileRefCDNURL+`"}}`, false)
		items := singleInsertItems(t, srvState)
		image, _ := items["Image"].(map[string]any)
		want := "nimbu://" + fileRefSiteShort + "/uploads/" + fileRefUploadID
		if image["__type"] != "FileRef" || image["source"] != want || len(image) != 2 {
			t.Fatalf("Image = %#v, want FileRef %s", items["Image"], want)
		}
	})

	t.Run("attachment_path is read from disk", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "img.webp")
		if err := os.WriteFile(path, []byte("webp-bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
		srvState := &surgicalServer{}
		pathJSON, _ := json.Marshal(path)
		runInsert(t, srvState, `{"Image":{"attachment_path":`+string(pathJSON)+`}}`, false)
		items := singleInsertItems(t, srvState)
		image, _ := items["Image"].(map[string]any)
		if image["data"] == nil || image["filename"] != "img.webp" || image["content_type"] == "" {
			t.Fatalf("Image = %#v", items["Image"])
		}
	})

	t.Run("dry-run prints one insert op carrying the file", func(t *testing.T) {
		asset := newAsset(t, nil)
		srvState := &surgicalServer{}
		out := runInsert(t, srvState, `{"Image":{"attachment_url":"`+asset.URL+`/a.jpg"}}`, true)
		if srvState.posts != 0 {
			t.Fatalf("dry-run posted %d times", srvState.posts)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(out), &body); err != nil {
			t.Fatal(err)
		}
		ops := body["operations"].([]any)
		if len(ops) != 1 || ops[0].(map[string]any)["op"] != "insert" {
			t.Fatalf("operations = %#v", ops)
		}
		image, _ := insertOpItems(t, body)["Image"].(map[string]any)
		if image["data"] == nil {
			t.Fatalf("dry-run Image = %#v", image)
		}
	})

	t.Run("412 retry reuses the downloaded file", func(t *testing.T) {
		hits := 0
		asset := newAsset(t, &hits)
		var etags []string
		srvState := &surgicalServer{}
		srvState.batchFn = func(w http.ResponseWriter, r *http.Request, n int) {
			etags = append(etags, r.Header.Get("If-Match"))
			if n == 1 {
				w.WriteHeader(http.StatusPreconditionFailed)
				_, _ = w.Write([]byte(`{"message":"Precondition Failed","code":"precondition_failed","current_etag":"deadbeef"}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"results":[{"index":0,"status":"ok","path":"/items/Blokken/repeatables","id":"newblock000000000000001"}],
				"etag":"etag-from-insert",
				"updated_at":"2026-09-10T16:00:00.000Z",
				"page":` + surgicalPageJSON() + `
			}`))
		}
		runInsert(t, srvState, `{"Image":{"attachment_url":"`+asset.URL+`/a.jpg"}}`, false)
		if srvState.posts != 2 || etags[1] != `"deadbeef"` {
			t.Fatalf("posts=%d etags=%v, want a single 412 retry", srvState.posts, etags)
		}
		if hits != 1 {
			t.Fatalf("asset downloaded %d times, want 1", hits)
		}
		image, _ := insertOpItems(t, srvState.lastBody)["Image"].(map[string]any)
		if image["data"] == nil {
			t.Fatalf("retried Image = %#v", image)
		}
	})
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

func surgicalNestedSchemaJSON() string {
	return `{
		"template":{"id":"t1","name":"home"},
		"available_blocks":{
			"Blokken":[
				{"slug":"hero_stage","label":"Hero","fields":[
					{"slug":"Title","type":"text"},
					{"slug":"Items","type":"canvas","available_blocks":[
						{"slug":"tile","label":"Tile","fields":[{"slug":"Label","type":"text"}]}
					]}
				]}
			]
		}
	}`
}

func TestPagesInsertAcceptsNestedCanvasPaths(t *testing.T) {
	wantPath := "/items/Blokken/repeatables/" + surgicalBlockID + "/items/Items/repeatables"

	for name, path := range map[string]string{
		"human":            "Blokken[id=" + surgicalBlockID + "].Items",
		"human by index":   "Blokken[0].Items",
		"raw":              "/items/Blokken/repeatables/" + surgicalBlockID + "/items/Items",
		"raw with index":   "/items/Blokken/repeatables/0/items/Items",
		"raw /repeatables": "/items/Blokken/repeatables/0/items/Items/repeatables",
	} {
		t.Run(name, func(t *testing.T) {
			srvState := &surgicalServer{pageJSON: surgicalNestedPageJSON(), schemaJSON: surgicalNestedSchemaJSON()}
			srv := srvState.start(t)
			defer srv.Close()
			ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
			cmd := &PagesInsertCmd{Page: "about", Path: path, Slug: "tile"}
			if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
				t.Fatalf("run: %v", err)
			}
			op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
			if op["op"] != "insert" || op["path"] != wantPath {
				t.Fatalf("op = %#v, want path %s", op, wantPath)
			}
		})
	}
}

func TestPagesInsertRawIndexOutOfRange(t *testing.T) {
	srvState := &surgicalServer{pageJSON: surgicalNestedPageJSON(), schemaJSON: surgicalNestedSchemaJSON()}
	srv := srvState.start(t)
	defer srv.Close()
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesInsertCmd{Page: "about", Path: "/items/Blokken/repeatables/11/items/Items", Slug: "tile"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "index 11") {
		t.Fatalf("error = %v", err)
	}
	if srvState.posts != 0 {
		t.Fatal("bad index should not post")
	}
}
