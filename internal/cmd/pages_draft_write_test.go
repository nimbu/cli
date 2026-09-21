package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestPagesSetDraftRoutesToDraftBatchWithoutIfMatch(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesSetCmd{Page: "about", Path: "Blokken[2].Quote", Value: "From draft", Draft: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if srvState.posts != 0 || srvState.draftPosts != 1 || srvState.draftGets != 1 {
		t.Fatalf("live=%d draftPosts=%d draftGets=%d", srvState.posts, srvState.draftPosts, srvState.draftGets)
	}
	if srvState.ifMatch[0] != "" {
		t.Fatalf("If-Match = %q", srvState.ifMatch[0])
	}
	if strings.Contains(srvState.queries[0], "atomic=") {
		t.Fatalf("draft query = %q", srvState.queries[0])
	}
	op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
	if op["path"] != "/items/Blokken/repeatables/"+surgicalDraftOnlyID+"/items/Quote" {
		t.Fatalf("resolved against live or wrong id: %#v", op)
	}
	if !strings.Contains(out.String(), "Updated draft of about (draft "+surgicalDraftID+")") {
		t.Fatalf("out = %q", out.String())
	}
	if !strings.Contains(out.String(), "nimbu pages draft preview-url --page about") {
		t.Fatalf("missing preview hint: %q", out.String())
	}
}

func TestPagesSetDraftJSONOmitsLiveETag(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesSetCmd{Page: "about", Path: "title", Value: "X", Draft: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["etag"]; ok {
		t.Fatalf("etag should be omitted: %#v", body)
	}
	draft, _ := body["draft"].(map[string]any)
	if draft["id"] != surgicalDraftID || draft["page_id"] != surgicalPageID {
		t.Fatalf("draft = %#v", draft)
	}
}

func TestPagesSetDraftFallsBackToLiveWhenNoDraft(t *testing.T) {
	srvState := &surgicalServer{noDraft: true}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesSetCmd{Page: "about", Path: "Blokken[0].Title", Value: "Hello", Draft: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if srvState.draftPosts != 1 {
		t.Fatalf("draftPosts = %d", srvState.draftPosts)
	}
	op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
	if op["path"] != "/items/Blokken/repeatables/"+surgicalBlockID+"/items/Title" {
		t.Fatalf("op = %#v", op)
	}
}

func TestPagesInsertMoveDeleteBlockDraftRouting(t *testing.T) {
	t.Run("insert", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesInsertCmd{Page: "about", Path: "Blokken", Slug: "hero_stage", Draft: true}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if srvState.posts != 0 || srvState.draftPosts == 0 {
			t.Fatalf("live=%d draft=%d", srvState.posts, srvState.draftPosts)
		}
		if srvState.ifMatch[0] != "" {
			t.Fatalf("If-Match = %q", srvState.ifMatch[0])
		}
	})

	t.Run("move", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		one := 1
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesMoveCmd{Page: "about", Path: "Blokken[0]", Position: &one, Draft: true}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if srvState.posts != 0 || srvState.draftPosts != 1 {
			t.Fatalf("live=%d draft=%d", srvState.posts, srvState.draftPosts)
		}
	})

	t.Run("delete-block", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesDeleteBlockCmd{Page: "about", Path: "Blokken[0]", Draft: true}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo", Force: true}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if srvState.posts != 0 || srvState.draftPosts != 1 {
			t.Fatalf("live=%d draft=%d", srvState.posts, srvState.draftPosts)
		}
	})

	t.Run("batch", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		file := writePageFile(t, `{"operations":[{"op":"set","path":"title","value":"Hi"}]}`)
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesBatchCmd{Page: "about", File: file, Atomic: true, Draft: true}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if srvState.posts != 0 || srvState.draftPosts != 1 {
			t.Fatalf("live=%d draft=%d", srvState.posts, srvState.draftPosts)
		}
		if srvState.ifMatch[0] != "" {
			t.Fatalf("If-Match = %q", srvState.ifMatch[0])
		}
	})
}

func TestPagesSetDraftDiffUnavailableMessage(t *testing.T) {
	srvState := &surgicalServer{
		draftJSON: `{
			"id":"` + surgicalDraftID + `",
			"page_id":"` + surgicalPageID + `",
			"updated_at":"` + surgicalDraftUpdated + `",
			"content":{"title":"no items"}
		}`,
	}
	srvState.draftBatchFn = func(w http.ResponseWriter, _ *http.Request, _ int) {
		_, _ = w.Write([]byte(`{
			"results":[{"index":0,"status":"ok","path":"/title"}],
			"draft":{"id":"` + surgicalDraftID + `","page_id":"` + surgicalPageID + `","updated_at":"` + surgicalDraftUpdated + `","content":{"title":"still no items"}}
		}`))
	}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesSetCmd{Page: "about", Path: "title", Value: "X", Draft: true, Diff: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "(diff unavailable for drafts)") {
		t.Fatalf("out = %q", out.String())
	}
}

func TestCommandContractContainsPageDrafts(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}
	paths := map[string]bool{}
	draftFlags := map[string]bool{}
	for _, command := range buildCommandContract(parser.Model).Commands {
		paths[command.Path] = true
		if command.Path == "nimbu pages set" || command.Path == "nimbu pages insert" ||
			command.Path == "nimbu pages delete-block" || command.Path == "nimbu pages move" ||
			command.Path == "nimbu pages batch" {
			for _, flag := range command.Flags {
				if flag.Name == "draft" {
					draftFlags[command.Path] = true
				}
			}
		}
	}
	for _, path := range []string{
		"nimbu pages draft get",
		"nimbu pages draft save",
		"nimbu pages draft batch",
		"nimbu pages draft publish",
		"nimbu pages draft discard",
		"nimbu pages draft preview-url",
	} {
		if !paths[path] {
			t.Errorf("command contract missing %q", path)
		}
	}
	for _, path := range []string{
		"nimbu pages set", "nimbu pages insert", "nimbu pages delete-block", "nimbu pages move", "nimbu pages batch",
	} {
		if !draftFlags[path] {
			t.Errorf("%s missing --draft", path)
		}
	}
}

func TestWarnDraftLocaleOnlyWhenBothSet(t *testing.T) {
	for _, tc := range []struct {
		draft  bool
		locale string
		want   bool
	}{{true, "en", true}, {true, "", false}, {false, "en", false}} {
		var stderr bytes.Buffer
		ctx := output.WithWriter(context.Background(), &output.Writer{Out: &bytes.Buffer{}, Err: &stderr})
		warnDraftLocale(ctx, tc.draft, tc.locale)
		if got := strings.Contains(stderr.String(), "--draft with --locale"); got != tc.want {
			t.Fatalf("draft=%v locale=%q: warning printed=%v want %v", tc.draft, tc.locale, got, tc.want)
		}
	}
}
