package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

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

func TestPagesSetTitleDiffShowsPageFieldChange(t *testing.T) {
	const beforeTitle = "CLI surgical test v2"
	const afterTitle = "CLI surgical test v3"
	srvState := &surgicalServer{
		pageJSON: strings.Replace(surgicalPageJSON(), `"title":"About"`, `"title":"`+beforeTitle+`"`, 1),
	}
	srvState.batchFn = func(w http.ResponseWriter, _ *http.Request, _ int) {
		page := strings.Replace(surgicalPageJSON(), `"title":"About"`, `"title":"`+afterTitle+`"`, 1)
		_, _ = w.Write([]byte(`{
			"results":[{"index":0,"status":"ok","path":"/title"}],
			"etag":"newetag12",
			"updated_at":"2026-09-10T15:00:00.000Z",
			"page":` + page + `
		}`))
	}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesSetCmd{Page: "about", Path: "title", Value: afterTitle, Diff: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "(no change)") {
		t.Fatalf("diff reported no change:\n%s", got)
	}
	if !strings.Contains(got, `"`+beforeTitle+`"`) || !strings.Contains(got, `"`+afterTitle+`"`) {
		t.Fatalf("diff missing title strings:\n%s", got)
	}
}
