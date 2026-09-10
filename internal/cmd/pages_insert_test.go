package cmd

import (
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
	srvState := &surgicalServer{}
	srvState.batchFn = func(w http.ResponseWriter, r *http.Request, n int) {
		etags = append(etags, r.Header.Get("If-Match"))
		if n == 1 {
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
