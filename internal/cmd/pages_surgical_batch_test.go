package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

const surgicalNestedID = "6a6c699f0655c6dcb4dd8551"

// surgicalNestedPageJSON has a canvas (Items) nested inside a repeatable.
func surgicalNestedPageJSON() string {
	return `{
		"id":"` + surgicalPageID + `",
		"fullpath":"about",
		"updated_at":"` + surgicalUpdatedAt + `",
		"title":"About",
		"items":{
			"Blokken":{
				"type":"canvas",
				"repeatables":[
					{
						"id":"` + surgicalBlockID + `",
						"slug":"hero_stage",
						"position":1,
						"items":{
							"Title":{"type":"text","content":"Hero"},
							"Items":{"type":"canvas","repeatables":[
								{"id":"` + surgicalNestedID + `","slug":"tile","position":1,"items":{"Label":{"type":"text","content":"One"}}}
							]}
						}
					}
				]
			}
		}
	}`
}

func runBatchFile(t *testing.T, srvState *surgicalServer, contents string, cmd *PagesBatchCmd) (string, error) {
	t.Helper()
	srv := srvState.start(t)
	defer srv.Close()
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd.Page = "about"
	cmd.File = writePageFile(t, contents)
	cmd.Atomic = true
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	return out.String(), err
}

func batchOpAt(t *testing.T, srvState *surgicalServer, i int) map[string]any {
	t.Helper()
	ops, ok := srvState.lastBody["operations"].([]any)
	if !ok || len(ops) <= i {
		t.Fatalf("operations = %#v", srvState.lastBody["operations"])
	}
	return ops[i].(map[string]any)
}

func TestPagesBatchInsertHumanCanvasPaths(t *testing.T) {
	t.Run("bare canvas name", func(t *testing.T) {
		srvState := &surgicalServer{}
		if _, err := runBatchFile(t, srvState,
			`[{"op":"insert","path":"Blokken","value":{"slug":"proof_strip","items":{}}}]`,
			&PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if got := batchOpAt(t, srvState, 0)["path"]; got != "/items/Blokken/repeatables" {
			t.Fatalf("insert path = %#v", got)
		}
	})

	t.Run("nested canvas", func(t *testing.T) {
		srvState := &surgicalServer{pageJSON: surgicalNestedPageJSON()}
		if _, err := runBatchFile(t, srvState,
			`[{"op":"insert","path":"Blokken[id=`+surgicalBlockID+`].Items","value":{"slug":"tile","items":{}}}]`,
			&PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		want := "/items/Blokken/repeatables/" + surgicalBlockID + "/items/Items/repeatables"
		if got := batchOpAt(t, srvState, 0)["path"]; got != want {
			t.Fatalf("insert path = %#v, want %s", got, want)
		}
	})

	t.Run("raw canvas path gains /repeatables", func(t *testing.T) {
		srvState := &surgicalServer{}
		if _, err := runBatchFile(t, srvState,
			`[{"op":"insert","path":"/items/Blokken","value":{"slug":"proof_strip","items":{}}}]`,
			&PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if got := batchOpAt(t, srvState, 0)["path"]; got != "/items/Blokken/repeatables" {
			t.Fatalf("insert path = %#v", got)
		}
	})

	t.Run("repeatable path is rejected", func(t *testing.T) {
		srvState := &surgicalServer{}
		_, err := runBatchFile(t, srvState,
			`[{"op":"insert","path":"Blokken[0]","value":{"slug":"proof_strip","items":{}}}]`,
			&PagesBatchCmd{})
		if err == nil || !strings.Contains(err.Error(), "expected canvas") {
			t.Fatalf("error = %v", err)
		}
		if srvState.posts != 0 {
			t.Fatal("invalid insert path should not post")
		}
	})
}

func TestPagesBatchRawIndexResolvesToRepeatableID(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		srvState := &surgicalServer{}
		if _, err := runBatchFile(t, srvState,
			`[{"op":"set","path":"/items/Blokken/repeatables/1/items/Quote","value":"Hi"}]`,
			&PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		want := "/items/Blokken/repeatables/" + surgicalBlockID2 + "/items/Quote"
		if got := batchOpAt(t, srvState, 0)["path"]; got != want {
			t.Fatalf("path = %#v, want %s", got, want)
		}
	})

	t.Run("delete and move", func(t *testing.T) {
		srvState := &surgicalServer{}
		if _, err := runBatchFile(t, srvState, `[
			{"op":"delete","path":"/items/Blokken/repeatables/0"},
			{"op":"move","path":"/items/Blokken/repeatables/1","after":null}
		]`, &PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if got := batchOpAt(t, srvState, 0)["path"]; got != "/items/Blokken/repeatables/"+surgicalBlockID {
			t.Fatalf("delete path = %#v", got)
		}
		if got := batchOpAt(t, srvState, 1)["path"]; got != "/items/Blokken/repeatables/"+surgicalBlockID2 {
			t.Fatalf("move path = %#v", got)
		}
	})

	t.Run("existing ids are untouched", func(t *testing.T) {
		srvState := &surgicalServer{}
		raw := "/items/Blokken/repeatables/" + surgicalBlockID + "/items/Title"
		if _, err := runBatchFile(t, srvState,
			`[{"op":"set","path":"`+raw+`","value":"Hi"}]`, &PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if got := batchOpAt(t, srvState, 0)["path"]; got != raw {
			t.Fatalf("path = %#v", got)
		}
	})

	t.Run("out of range index fails before posting", func(t *testing.T) {
		srvState := &surgicalServer{}
		_, err := runBatchFile(t, srvState,
			`[{"op":"set","path":"/items/Blokken/repeatables/11/items/Quote","value":"Hi"}]`,
			&PagesBatchCmd{})
		if err == nil {
			t.Fatal("expected index error")
		}
		for _, needle := range []string{"index 11", "Blokken", surgicalBlockID} {
			if !strings.Contains(err.Error(), needle) {
				t.Fatalf("error %v missing %q", err, needle)
			}
		}
		if srvState.posts != 0 {
			t.Fatal("bad index should not post")
		}
	})

	t.Run("dry run resolves the index too", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
		file := writePageFile(t, `[{"op":"set","path":"/items/Blokken/repeatables/1/items/Quote","value":"Hi"}]`)
		cmd := &PagesBatchCmd{Page: "about", File: file, Atomic: true, DryRun: true}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if srvState.posts != 0 {
			t.Fatal("dry run should not post")
		}
		var body dryRunOutput
		if err := json.Unmarshal(out.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		want := "/items/Blokken/repeatables/" + surgicalBlockID2 + "/items/Quote"
		if body.Operations[0].Path != want {
			t.Fatalf("dry run path = %q, want %q", body.Operations[0].Path, want)
		}
	})

	t.Run("dry run rejects an out of range index", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		file := writePageFile(t, `[{"op":"set","path":"/items/Blokken/repeatables/11/items/Quote","value":"Hi"}]`)
		cmd := &PagesBatchCmd{Page: "about", File: file, Atomic: true, DryRun: true}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err == nil {
			t.Fatal("expected dry-run index error")
		}
	})
}

func TestPagesBatchRejectsBadOpsClientSide(t *testing.T) {
	t.Run("unknown op", func(t *testing.T) {
		srvState := &surgicalServer{}
		_, err := runBatchFile(t, srvState,
			`[{"op":"replace","path":"/title","value":"Hi"}]`, &PagesBatchCmd{})
		if err == nil {
			t.Fatal("expected unknown op error")
		}
		for _, needle := range []string{`unknown op "replace"`, "set, insert, delete, move"} {
			if !strings.Contains(err.Error(), needle) {
				t.Fatalf("error %v missing %q", err, needle)
			}
		}
		if srvState.posts != 0 {
			t.Fatal("unknown op should not post")
		}
	})

	t.Run("content suffix", func(t *testing.T) {
		srvState := &surgicalServer{}
		_, err := runBatchFile(t, srvState,
			`[{"op":"set","path":"/items/Blokken/repeatables/`+surgicalBlockID+`/items/Title/content","value":"Hi"}]`,
			&PagesBatchCmd{})
		if err == nil || !strings.Contains(err.Error(), "paths end at the editable name") {
			t.Fatalf("error = %v", err)
		}
		if srvState.posts != 0 {
			t.Fatal("content suffix should not post")
		}
	})
}

func TestPagesBatchHumanAndJSONResults(t *testing.T) {
	t.Run("human prints per-op result lines", func(t *testing.T) {
		srvState := &surgicalServer{}
		out, err := runBatchFile(t, srvState,
			`[{"op":"set","path":"Blokken[0].Title","value":"Hi"}]`, &PagesBatchCmd{})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		line := strings.SplitN(out, "\n", 2)[0]
		if !strings.HasPrefix(line, "ok ") || !strings.Contains(line, "set") ||
			!strings.Contains(line, "/items/Blokken/repeatables/"+surgicalBlockID+"/items/Title") {
			t.Fatalf("first line = %q (full output %q)", line, out)
		}
	})

	t.Run("json carries the results array", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
		file := writePageFile(t, `[{"op":"set","path":"Blokken[0].Title","value":"Hi"}]`)
		cmd := &PagesBatchCmd{Page: "about", File: file, Atomic: true}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		var body surgicalJSONResult
		if err := json.Unmarshal(out.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Results) != 1 || body.Results[0].Status != "ok" {
			t.Fatalf("results = %#v", body.Results)
		}
	})
}

func TestPagesBatchHelpDocumentsOpsAndExample(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")
	t.Setenv("NIMBU_COLOR", "never")

	code, stdout, stderr := captureExecute(t, []string{"pages", "batch", "--help"})
	if code != 0 {
		t.Fatalf("pages batch --help exit %d stderr=%q", code, stderr)
	}
	for _, needle := range []string{
		`{"operations":[...]}`,
		"bare array",
		"set     path, value",
		"insert  path, value",
		"delete  path",
		"move    path, after",
		"omit it to append, null to place first",
		"never append /content",
		`{"op":"set","path":"Blokken[0].Title","value":"Hello"}`,
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("pages batch --help missing %q\n%s", needle, stdout)
		}
	}
}

func TestPagesBatchDraftRejectsNoAtomic(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "http://127.0.0.1:1", output.Mode{JSON: true})
	cmd := &PagesBatchCmd{
		Page:   "about",
		File:   writePageFile(t, `[{"op":"set","path":"/title","value":"Hi"}]`),
		Draft:  true,
		Atomic: false,
	}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected --draft --no-atomic to be rejected")
	}
	if !strings.Contains(err.Error(), "--no-atomic cannot be combined with --draft") ||
		!strings.Contains(err.Error(), "always atomic") {
		t.Fatalf("err = %v", err)
	}
	desc := classifyError(err)
	if desc.Code != errorRequestInvalid || desc.ExitCode != ExitUsage {
		t.Fatalf("desc = %#v", desc)
	}

	// --draft on its own (atomic, the default) still runs.
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()
	ctxOK, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd.Atomic = true
	if err := cmd.Run(ctxOK, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("draft batch: %v", err)
	}
	if srvState.draftPosts != 1 {
		t.Fatalf("draftPosts = %d", srvState.draftPosts)
	}
}

func TestValidateBatchOpPathOnlyRejectsContentPropertySegment(t *testing.T) {
	rejected := []string{
		"/items/Title/content",
		"/items/Blokken/repeatables/" + surgicalBlockID + "/items/Title/content",
		"/items/content/content",
	}
	for _, path := range rejected {
		err := validateBatchOpPath(path)
		if err == nil {
			t.Fatalf("path %q should be rejected", path)
		}
		if !strings.Contains(err.Error(), "ends in /content") {
			t.Fatalf("path %q error = %v", path, err)
		}
	}
	allowed := []string{
		"/items/content",
		"/items/Blokken/repeatables/" + surgicalBlockID + "/items/content",
		"/items/Title",
		"/title",
		"/content",
	}
	for _, path := range allowed {
		if err := validateBatchOpPath(path); err != nil {
			t.Fatalf("path %q should be allowed, got %v", path, err)
		}
	}
}

func TestPagesBatchAllowsEditableNamedContent(t *testing.T) {
	t.Run("editable named content is posted verbatim", func(t *testing.T) {
		srvState := &surgicalServer{}
		if _, err := runBatchFile(t, srvState,
			`[{"op":"set","path":"/items/content","value":"Hi"}]`,
			&PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if got := batchOpAt(t, srvState, 0)["path"]; got != "/items/content" {
			t.Fatalf("path = %v", got)
		}
	})

	t.Run("nested editable named content is posted verbatim", func(t *testing.T) {
		raw := "/items/Blokken/repeatables/" + surgicalBlockID + "/items/content"
		srvState := &surgicalServer{}
		if _, err := runBatchFile(t, srvState,
			`[{"op":"set","path":"`+raw+`","value":"Hi"}]`,
			&PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if got := batchOpAt(t, srvState, 0)["path"]; got != raw {
			t.Fatalf("path = %v", got)
		}
	})

	t.Run("a /content property segment is still rejected", func(t *testing.T) {
		srvState := &surgicalServer{}
		_, err := runBatchFile(t, srvState,
			`[{"op":"set","path":"/items/Blokken/repeatables/`+surgicalBlockID+`/items/Title/content","value":"Hi"}]`,
			&PagesBatchCmd{})
		if err == nil || !strings.Contains(err.Error(), "ends in /content") {
			t.Fatalf("err = %v", err)
		}
		if srvState.posts != 0 {
			t.Fatalf("nothing should be posted, posts = %d", srvState.posts)
		}
	})
}

// batchRetryPageJSON renders the page with the two blocks in the given order,
// so a reload can hand back a different positional ordering.
func batchRetryPageJSON(firstID, secondID string) string {
	block := func(id, slug, title string, position int) string {
		return `{"id":"` + id + `","slug":"` + slug + `","position":` + strconv.Itoa(position) + `,"items":{"Title":{"type":"text","content":"` + title + `"}}}`
	}
	return `{
		"id":"` + surgicalPageID + `",
		"fullpath":"about",
		"updated_at":"` + surgicalUpdatedAt + `",
		"title":"About",
		"items":{
			"Blokken":{"type":"canvas","repeatables":[
				` + block(firstID, "hero_stage", "First", 1) + `,
				` + block(secondID, "proof_strip", "Second", 2) + `
			]}
		}
	}`
}

// TestPagesBatchRetriesOnceOn412AndReresolvesIndexes covers pages batch's own
// 412 path: the repeatable order changes between the two reads, so the raw
// index path must resolve to a different repeatable id on the retry.
func TestPagesBatchRetriesOnceOn412AndReresolvesIndexes(t *testing.T) {
	var gets, posts int
	var postedPaths, ifMatch []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/pages/about" || r.URL.Path == "/pages/"+surgicalPageID):
			gets++
			if gets == 1 {
				_, _ = w.Write([]byte(batchRetryPageJSON(surgicalBlockID, surgicalBlockID2)))
				return
			}
			// Somebody reordered the canvas between the two reads.
			_, _ = w.Write([]byte(batchRetryPageJSON(surgicalBlockID2, surgicalBlockID)))
		case r.Method == http.MethodPost && r.URL.Path == "/pages/"+surgicalPageID+"/batch":
			posts++
			ifMatch = append(ifMatch, r.Header.Get("If-Match"))
			body, _ := io.ReadAll(r.Body)
			var decoded map[string]any
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatalf("decode batch body: %v", err)
			}
			ops, _ := decoded["operations"].([]any)
			if len(ops) != 1 {
				t.Fatalf("operations = %#v", decoded["operations"])
			}
			op, _ := ops[0].(map[string]any)
			postedPaths = append(postedPaths, stringAny(op["path"]))
			if posts == 1 {
				w.WriteHeader(http.StatusPreconditionFailed)
				_, _ = w.Write([]byte(`{"message":"Precondition Failed","code":"precondition_failed","current_etag":"deadbeef"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"results":[{"index":0,"status":"ok","path":"` + stringAny(op["path"]) + `"}],
				"etag":"newetag12",
				"updated_at":"2026-09-10T15:00:00.000Z",
				"page":` + batchRetryPageJSON(surgicalBlockID2, surgicalBlockID) + `
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesBatchCmd{
		Page:   "about",
		File:   writePageFile(t, `[{"op":"set","path":"/items/Blokken/repeatables/0/items/Title","value":"Hi"}]`),
		Atomic: true,
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if posts != 2 || gets != 2 {
		t.Fatalf("gets=%d posts=%d, want 2 and 2", gets, posts)
	}
	wantFirst := "/items/Blokken/repeatables/" + surgicalBlockID + "/items/Title"
	wantRetry := "/items/Blokken/repeatables/" + surgicalBlockID2 + "/items/Title"
	if postedPaths[0] != wantFirst {
		t.Fatalf("first attempt path = %q, want %q", postedPaths[0], wantFirst)
	}
	if postedPaths[1] != wantRetry {
		t.Fatalf("retry path = %q, want %q (index 0 must re-resolve after the reorder)", postedPaths[1], wantRetry)
	}
	if ifMatch[1] != `"deadbeef"` {
		t.Fatalf("retry If-Match = %q", ifMatch[1])
	}
	var body surgicalJSONResult
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Results) != 1 || body.Results[0].Status != "ok" {
		t.Fatalf("results = %#v", body.Results)
	}
}
