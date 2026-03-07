package apps

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/config"
)

func TestPlanPushAndExecutePushHandleCreateUpdateDelete(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "code"), 0o755); err != nil {
		t.Fatalf("mkdir code: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "code", "main.js"), []byte(`require("./shared")`), 0o644); err != nil {
		t.Fatalf("write main.js: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "code", "shared.js"), []byte(`module.exports = 1`), 0o644); err != nil {
		t.Fatalf("write shared.js: %v", err)
	}

	app := AppConfig{
		AppProjectConfig: config.AppProjectConfig{ID: "storefront", Name: "storefront", Dir: "code", Glob: "**/*.js"},
		ProjectRoot:      root,
	}

	var ops []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apps/storefront/code":
			_, _ = w.Write([]byte(`[{"name":"main.js"},{"name":"orphan.js"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/apps/storefront/code":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode post: %v", err)
			}
			ops = append(ops, "create:"+body["name"].(string))
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPut && r.URL.Path == "/apps/storefront/code/main.js":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode put: %v", err)
			}
			ops = append(ops, "update:main.js")
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/apps/storefront/code/orphan.js":
			ops = append(ops, "delete:orphan.js")
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := api.New(srv.URL, "token")
	files, err := CollectFiles(app)
	if err != nil {
		t.Fatalf("collect files: %v", err)
	}
	ordered, err := OrderFiles(app, files)
	if err != nil {
		t.Fatalf("order files: %v", err)
	}
	result, deletes, err := PlanPush(context.Background(), client, app, ordered, true)
	if err != nil {
		t.Fatalf("plan push: %v", err)
	}
	if len(result.Created) != 1 || result.Created[0].Name != "shared.js" {
		t.Fatalf("unexpected creates: %#v", result.Created)
	}
	if len(result.Updated) != 1 || result.Updated[0].Name != "main.js" {
		t.Fatalf("unexpected updates: %#v", result.Updated)
	}
	if len(deletes) != 1 || deletes[0] != "orphan.js" {
		t.Fatalf("unexpected deletes: %#v", deletes)
	}

	if err := ExecutePush(context.Background(), client, app, result); err != nil {
		t.Fatalf("execute push: %v", err)
	}
	got := strings.Join(ops, ",")
	if got != "create:shared.js,update:main.js,delete:orphan.js" {
		t.Fatalf("unexpected ops: %s", got)
	}
}

func TestExecutePushPreservesTopologicalUploadOrderAcrossCreateAndUpdate(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "code"), 0o755); err != nil {
		t.Fatalf("mkdir code: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "code", "main.js"), []byte(`require("./shared")`), 0o644); err != nil {
		t.Fatalf("write main.js: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "code", "shared.js"), []byte(`module.exports = 1`), 0o644); err != nil {
		t.Fatalf("write shared.js: %v", err)
	}

	app := AppConfig{
		AppProjectConfig: config.AppProjectConfig{ID: "storefront", Name: "storefront", Dir: "code", Glob: "**/*.js"},
		ProjectRoot:      root,
	}

	var ops []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apps/storefront/code":
			_, _ = w.Write([]byte(`[{"name":"shared.js"}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/apps/storefront/code/shared.js":
			ops = append(ops, "update:shared.js")
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPost && r.URL.Path == "/apps/storefront/code":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode post: %v", err)
			}
			ops = append(ops, "create:"+body["name"].(string))
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := api.New(srv.URL, "token")
	files, err := CollectFiles(app)
	if err != nil {
		t.Fatalf("collect files: %v", err)
	}
	ordered, err := OrderFiles(app, files)
	if err != nil {
		t.Fatalf("order files: %v", err)
	}
	result, _, err := PlanPush(context.Background(), client, app, ordered, false)
	if err != nil {
		t.Fatalf("plan push: %v", err)
	}
	if len(result.Uploads) != 2 {
		t.Fatalf("upload count = %d", len(result.Uploads))
	}
	if result.Uploads[0].Name != "shared.js" || result.Uploads[0].Action != "update" {
		t.Fatalf("unexpected first upload: %#v", result.Uploads[0])
	}
	if result.Uploads[1].Name != "main.js" || result.Uploads[1].Action != "create" {
		t.Fatalf("unexpected second upload: %#v", result.Uploads[1])
	}

	if err := ExecutePush(context.Background(), client, app, result); err != nil {
		t.Fatalf("execute push: %v", err)
	}
	if got := strings.Join(ops, ","); got != "update:shared.js,create:main.js" {
		t.Fatalf("unexpected ops: %s", got)
	}
}
