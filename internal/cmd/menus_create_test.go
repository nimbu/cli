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

func TestMenusCreateFromFilePreservesNestingAndStripsTargetPage(t *testing.T) {
	var gotMethod string
	var gotReplace string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/menus" {
			gotMethod = r.Method
			gotReplace = r.URL.Query().Get("replace")
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode post body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"m1","slug":"main","handle":"main","name":"Main"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	file := writeMenuFile(t, `{
		"name": "Main",
		"handle": "main",
		"items": [
			{
				"title": "Home",
				"target_page": "home",
				"url": "/"
			},
			{
				"title": "Shop",
				"children": [
					{"title": "Wine", "target_page": "wine", "url": "/wine"},
					{
						"title": "Gifts",
						"children": [
							{"title": "Boxes"}
						]
					}
				]
			}
		]
	}`)

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &MenusCreateCmd{File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run menus create: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %q", gotMethod)
	}
	if gotReplace != "" {
		t.Fatalf("expected no replace param on create, got %q", gotReplace)
	}

	items, ok := gotBody["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 root items, got %#v", gotBody["items"])
	}

	home := items[0].(map[string]any)
	if _, ok := home["target_page"]; ok {
		t.Fatalf("expected target_page stripped from root item, got %#v", home)
	}
	if home["url"] != "/" {
		t.Fatalf("expected unrelated fields preserved, got %#v", home["url"])
	}

	shop := items[1].(map[string]any)
	children, ok := shop["children"].([]any)
	if !ok || len(children) != 2 {
		t.Fatalf("expected 2 children preserved under Shop, got %#v", shop["children"])
	}

	wine := children[0].(map[string]any)
	if wine["title"] != "Wine" {
		t.Fatalf("expected first child Wine (nesting order preserved), got %#v", wine["title"])
	}
	if _, ok := wine["target_page"]; ok {
		t.Fatalf("expected target_page stripped from child item, got %#v", wine)
	}

	gifts := children[1].(map[string]any)
	grandchildren, ok := gifts["children"].([]any)
	if !ok || len(grandchildren) != 1 {
		t.Fatalf("expected grandchild preserved under Gifts, got %#v", gifts["children"])
	}
	if grandchildren[0].(map[string]any)["title"] != "Boxes" {
		t.Fatalf("expected grandchild Boxes, got %#v", grandchildren[0])
	}
}

func TestMenusCreateRejectsFileAndInlineAssignments(t *testing.T) {
	file := writeMenuFile(t, `{"name":"Main","handle":"main"}`)
	ctx, _, _ := newContractTestContext(t, "http://127.0.0.1:1", output.Mode{JSON: true})
	cmd := &MenusCreateCmd{File: file, Assignments: []string{"name=Override"}}

	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "use either --file or inline assignments") {
		t.Fatalf("expected file/inline XOR error, got %v", err)
	}
}

func writeMenuFile(t *testing.T, body string) string {
	t.Helper()
	path := t.TempDir() + "/menu.json"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write menu file: %v", err)
	}
	return path
}
