package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestMenusUpdateFromFilePreservesNestingWithReplace(t *testing.T) {
	var gotMethod string
	var gotReplace string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/menus/main" {
			_, _ = w.Write([]byte(`{
				"id":"m1","slug":"main","handle":"main","name":"Main",
				"items":[{"id":"old-item","title":"Old"}]
			}`))
			return
		}
		if r.Method == http.MethodPatch && r.URL.Path == "/menus/main" {
			gotMethod = r.Method
			gotReplace = r.URL.Query().Get("replace")
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			_, _ = w.Write([]byte(`{
				"id":"m1","slug":"main","handle":"main","name":"Main",
				"items":[
					{"title":"Home"},
					{"title":"Shop","children":[
						{"title":"Wine"},
						{"title":"Gifts","children":[{"title":"Boxes"}]}
					]}
				]
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	file := writeMenuFile(t, `{
		"name": "Main",
		"handle": "main",
		"items": [
			{"title": "Home", "target_page": "home", "url": "/"},
			{
				"title": "Shop",
				"children": [
					{"title": "Wine", "url": "/wine"},
					{"title": "Gifts", "children": [{"title": "Boxes"}]}
				]
			}
		]
	}`)

	ctx, _, errOut := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &MenusUpdateCmd{Menu: "main", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run menus update: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Fatalf("expected PATCH, got %q", gotMethod)
	}
	if gotReplace != "" {
		t.Fatalf("expected client-side reconciliation without replace=1, got %q", gotReplace)
	}

	items, ok := gotBody["items"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("expected 2 root items plus a tombstone, got %#v", gotBody["items"])
	}
	home := items[0].(map[string]any)
	if _, ok := home["target_page"]; ok {
		t.Fatalf("expected target_page stripped, got %#v", home)
	}
	if home["name"] != "Home" || home["target_url"] != "/" || home["kind"] != "url" {
		t.Fatalf("expected aliases and kind filled on update body, got %#v", home)
	}
	shop := items[1].(map[string]any)
	children := shop["children"].([]any)
	if len(children) != 2 {
		t.Fatalf("expected nested children preserved, got %#v", shop["children"])
	}
	gifts := children[1].(map[string]any)
	if len(gifts["children"].([]any)) != 1 {
		t.Fatalf("expected grandchild preserved, got %#v", gifts["children"])
	}
	tombstone := items[2].(map[string]any)
	if tombstone["id"] != "old-item" || tombstone["_destroy"] != true {
		t.Fatalf("expected removed item tombstone, got %#v", tombstone)
	}
	if strings.Contains(errOut.String(), "warning:") {
		t.Fatalf("expected no nesting warning, got %q", errOut.String())
	}
}

func TestMenusUpdateRejectsDeepInlineAssignments(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "http://127.0.0.1:1", output.Mode{})
	cmd := &MenusUpdateCmd{
		Menu:        "main",
		Assignments: []string{"items:=[]"},
	}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "requires --file or stdin") {
		t.Fatalf("expected shallow validator error, got %v", err)
	}
}

func TestMenusUpdateFromFileStopsWhenServerOmitsItemIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const menu = `{
			"id":"m1","slug":"main","handle":"main","name":"Main",
			"items":[{"name":"Visie","kind":"url","target_url":"#visie","children":[]}]
		}`
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/menus/main":
			_, _ = w.Write([]byte(menu))
		case r.Method == http.MethodGet && r.URL.Path == "/menus":
			_, _ = w.Write([]byte("[" + menu + "]"))
		default:
			t.Errorf("unexpected %s %s: menu must not be written", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusTeapot)
		}
	}))
	defer srv.Close()

	file := writeMenuFile(t, `{"items":[{"name":"Visie","url":"#visie"}]}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	err := (&MenusUpdateCmd{Menu: "main", File: file}).Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "item ids") || !strings.Contains(err.Error(), "no changes were written") {
		t.Fatalf("expected missing item ids error, got %v", err)
	}
}

func TestMenusUpdateRejectsInvalidKindBeforeAnyRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected %s %s: invalid kind must fail locally", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	file := writeMenuFile(t, `{"items":[
		{"name":"Home","url":"/"},
		{"name":"Shop","kind":"page","target_fullpath":"shop","children":[{"name":"Wine","kind":"foo"}]}
	]}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	err := (&MenusUpdateCmd{Menu: "main", File: file}).Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "items[1].children[0].kind") {
		t.Fatalf("expected item path in kind error, got %v", err)
	}
}
