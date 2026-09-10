package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/pagepath"
)

const (
	surgicalPageID    = "6a6c60362e4510dd4a459982"
	surgicalUpdatedAt = "2026-09-10T14:31:12.408Z"
	surgicalBlockID   = "6a6c699f0655c6dcb4dd854c"
	surgicalBlockID2  = "6a6c699f0655c6dcb4dd854d"
)

func surgicalPageETag(t *testing.T) string {
	t.Helper()
	etag, err := api.PageETag(surgicalPageID, surgicalUpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	return etag
}

func surgicalPageJSON() string {
	return `{
		"id":"` + surgicalPageID + `",
		"fullpath":"about",
		"updated_at":"` + surgicalUpdatedAt + `",
		"title":"About",
		"published":true,
		"items":{
			"Theme":{"type":"select","content":"Light"},
			"Blokken":{
				"type":"canvas",
				"repeatables":[
					{
						"id":"` + surgicalBlockID + `",
						"slug":"hero_stage",
						"position":1,
						"items":{
							"Title":{"type":"text","content":"Hero"},
							"Enabled":{"type":"switch","content":false},
							"Image":{"type":"file","file":{"url":"https://cdn.example.test/a.jpg"}},
							"Ref":{"type":"reference"}
						}
					},
					{
						"id":"` + surgicalBlockID2 + `",
						"slug":"proof_strip",
						"position":2,
						"items":{"Quote":{"type":"text","content":"Hi"}}
					}
				]
			}
		}
	}`
}

func surgicalSchemaJSON() string {
	return `{
		"template":{"id":"t1","name":"home"},
		"available_blocks":{
			"Blokken":[
				{"slug":"hero_stage","label":"Hero","fields":[
					{"slug":"Title","type":"text"},
					{"slug":"Enabled","type":"switch"},
					{"slug":"Image","type":"file"},
					{"slug":"Ref","type":"reference"}
				]},
				{"slug":"proof_strip","label":"Proof","fields":[
					{"slug":"Quote","type":"text"}
				]}
			]
		},
		"select_options":{
			"Theme":[{"label":"Light","value":"light"},{"label":"Dark","value":"dark"},{"label":"Auto","value":"auto"}]
		}
	}`
}

type surgicalServer struct {
	gets     int
	posts    int
	lastReq  *http.Request
	lastBody map[string]any
	ifMatch  []string
	queries  []string
	batchFn  func(http.ResponseWriter, *http.Request, int)
}

func (s *surgicalServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/pages/about" || r.URL.Path == "/pages/"+surgicalPageID):
			s.gets++
			_, _ = w.Write([]byte(surgicalPageJSON()))
		case r.Method == http.MethodGet && r.URL.Path == "/pages/"+surgicalPageID+"/schema":
			_, _ = w.Write([]byte(surgicalSchemaJSON()))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/pages/"+surgicalPageID+"/items/"):
			_, _ = w.Write([]byte(`{"path":"/items/Blokken/repeatables/` + surgicalBlockID + `/items/Title","parent_path":"/items/Blokken/repeatables/` + surgicalBlockID + `","position":1,"siblings_count":3,"type":"item","data":{"type":"text","content":"Hero"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/pages/"+surgicalPageID+"/batch":
			s.posts++
			s.lastReq = r
			s.ifMatch = append(s.ifMatch, r.Header.Get("If-Match"))
			s.queries = append(s.queries, r.URL.RawQuery)
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &s.lastBody); err != nil {
				t.Fatalf("decode batch body: %v", err)
			}
			if s.batchFn != nil {
				s.batchFn(w, r, s.posts)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"results":[{"index":0,"status":"ok","path":"/items/Blokken/repeatables/` + surgicalBlockID + `/items/Title","id":"` + surgicalBlockID + `"}],
				"etag":"newetag12",
				"updated_at":"2026-09-10T15:00:00.000Z",
				"page":` + surgicalPageJSON() + `
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestPagesSetHumanPathAndETag(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesSetCmd{Page: "about", Path: "Blokken[0].Title", Value: "Hello", Locale: "en"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages set: %v", err)
	}
	if srvState.posts != 1 || srvState.gets != 1 {
		t.Fatalf("gets=%d posts=%d", srvState.gets, srvState.posts)
	}
	if got := srvState.ifMatch[0]; got != `"`+surgicalPageETag(t)+`"` {
		t.Fatalf("If-Match = %s, want quoted page etag", got)
	}
	if !strings.Contains(srvState.queries[0], "atomic=1") || !strings.Contains(srvState.queries[0], "include=result") {
		t.Fatalf("query = %q", srvState.queries[0])
	}
	if !strings.Contains(srvState.queries[0], "content_locale=en") {
		t.Fatalf("missing content_locale: %q", srvState.queries[0])
	}
	ops := srvState.lastBody["operations"].([]any)
	op := ops[0].(map[string]any)
	if op["op"] != "set" || op["path"] != "/items/Blokken/repeatables/"+surgicalBlockID+"/items/Title" {
		t.Fatalf("op = %#v", op)
	}
	if op["value"] != "Hello" {
		t.Fatalf("value = %#v", op["value"])
	}
}

func TestPagesSetSwitchCoercionAndFileSources(t *testing.T) {
	t.Run("switch", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesSetCmd{Page: "about", Path: "Blokken[0].Enabled", Value: "yes"}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
		if op["value"] != true {
			t.Fatalf("switch value = %#v", op["value"])
		}
	})

	t.Run("file passthrough", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		file := writePageFile(t, `{"__type":"FileRef","source":"nimbu://uploads/1"}`)
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesSetCmd{Page: "about", Path: "Blokken[0].Image", File: file}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
		value := op["value"].(map[string]any)
		if value["__type"] != "FileRef" || value["source"] != "nimbu://uploads/1" {
			t.Fatalf("file value = %#v", value)
		}
	})

	t.Run("from-file", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		path := filepath.Join(t.TempDir(), "img.webp")
		if err := os.WriteFile(path, []byte("webp-bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesSetCmd{Page: "about", Path: "Blokken[0].Image", FromFile: path}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
		value := op["value"].(map[string]any)
		if value["filename"] != "img.webp" || value["data"] == nil || value["content_type"] == "" {
			t.Fatalf("from-file value = %#v", value)
		}
	})
}

func TestPagesSetRetriesOnceOn412(t *testing.T) {
	srvState := &surgicalServer{}
	srvState.batchFn = func(w http.ResponseWriter, _ *http.Request, n int) {
		if n == 1 {
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = w.Write([]byte(`{"message":"Precondition Failed","code":"precondition_failed","current_etag":"deadbeef"}`))
			return
		}
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"message":"Precondition Failed","code":"precondition_failed","current_etag":"deadbeef"}`))
	}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesSetCmd{Page: "about", Path: "title", Value: "X"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected 412")
	}
	if srvState.posts != 2 || srvState.gets != 2 {
		t.Fatalf("gets=%d posts=%d, want 2 and 2", srvState.gets, srvState.posts)
	}
	if srvState.ifMatch[1] != `"deadbeef"` {
		t.Fatalf("retry If-Match = %s", srvState.ifMatch[1])
	}
	desc := classifyError(err)
	if desc.HTTPStatus != 412 {
		t.Fatalf("http_status = %d", desc.HTTPStatus)
	}
	var exitErr *ExitError
	if !errors.As(emitCommandError(ctx, err), &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
}

func TestPagesSetAtomicFailureSurfacesResults(t *testing.T) {
	srvState := &surgicalServer{}
	srvState.batchFn = func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"message":"Atomic batch failed",
			"code":"atomic_failure",
			"results":[
				{"index":0,"status":"error","path":"/title","error":{"code":"invalid","message":"nope"}}
			]
		}`))
	}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesSetCmd{Page: "about", Path: "title", Value: "X"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected 422")
	}
	desc := classifyError(err)
	if desc.HTTPStatus != 422 {
		t.Fatalf("http_status = %d", desc.HTTPStatus)
	}
	results, ok := desc.Details["results"].([]api.BatchOpResult)
	if !ok || len(results) != 1 || results[0].Error == nil {
		t.Fatalf("details.results = %#v", desc.Details)
	}
}

func TestPagesSetResolveErrorIncludesCandidates(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesSetCmd{Page: "about", Path: "Blokken[9].Title", Value: "X"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected resolve error")
	}
	desc := classifyError(err)
	if desc.Code != errorRequestInvalid || desc.ExitCode != ExitUsage {
		t.Fatalf("code=%s exit=%d", desc.Code, desc.ExitCode)
	}
	cands, ok := desc.Details["candidates"].([]string)
	if !ok || len(cands) == 0 {
		t.Fatalf("candidates = %#v", desc.Details)
	}
	var exitErr *ExitError
	if !errors.As(emitCommandError(ctx, err), &exitErr) || exitErr.Code != 2 {
		t.Fatalf("exit = %#v", exitErr)
	}
}

func TestPagesSetDryRunPrintsOperationsAndSkipsPOST(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesSetCmd{Page: "about", Path: "Blokken[0].Title", Value: "Hello", DryRun: true}
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
	if ops[0].(map[string]any)["path"] != "/items/Blokken/repeatables/"+surgicalBlockID+"/items/Title" {
		t.Fatalf("dry-run op = %#v", ops[0])
	}
}

func TestCoerceSetValue(t *testing.T) {
	got, err := coerceSetValue(pagepath.Resolved{Type: "switch", RawPath: "/items/X"}, "1")
	if err != nil || got != true {
		t.Fatalf("switch 1 = %#v %v", got, err)
	}
	if _, err := coerceSetValue(pagepath.Resolved{Type: "switch"}, "maybe"); err == nil {
		t.Fatal("expected switch error")
	}
	got, err = coerceSetValue(pagepath.Resolved{Type: "reference", RawPath: "/items/R"}, "abc")
	if err != nil || got != "abc" {
		t.Fatalf("reference = %#v %v", got, err)
	}
}
