package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestSchemaPlanExit(t *testing.T) {
	for _, tc := range []struct {
		name, kind, risk string
		want             int
	}{
		{"safe", "add_field", "safe", 2}, {"destructive", "toggle_localized", "destructive", 3}, {"blocked", "retype_field", "blocked", 1}, {"unknown kind", "new_server_op", "safe", 1}, {"unknown risk", "add_field", "new_risk", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := schemaPlanExit([]schemaPlan{{Ops: []map[string]any{{"kind": tc.kind, "risk": tc.risk}}}}); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
	if schemaPlanExit(nil) != 0 {
		t.Fatal("empty plan must exit 0")
	}
}

func TestSchemaLoadPreservesUnknownKeysAndOrdersDependencies(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "schema", "channels")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"events": "slug: events\nfuture_key: preserved\nfields:\n - name: venue\n   type: belongs_to\n   reference: venues\n", "venues": "slug: venues\nfields: []\n"} {
		if err := os.WriteFile(filepath.Join(dir, name+".yml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	docs, err := loadSchemaDocuments(root)
	if err != nil {
		t.Fatal(err)
	}
	if docs[0].Target != "channel:venues" || docs[1].Body["future_key"] != "preserved" {
		t.Fatalf("unexpected documents: %#v", docs)
	}
}

func TestSchemaLoadRejectsMissingFieldsAndControls(t *testing.T) {
	for _, body := range []string{"name: products\n", "fields: []\nconfirm_destructive: true\n"} {
		root := t.TempDir()
		dir := filepath.Join(root, "schema")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "products.yml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadSchemaDocuments(root); err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}

func TestSchemaApplyStopsOnFailureAndCarriesGuard(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["fingerprint"] != "expected" || body["prune"] != true || body["confirm_destructive"] != true {
			t.Errorf("missing guard: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "second") {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"stale plan"}`))
			return
		}
		_, _ = w.Write([]byte(`{"applied":true,"fingerprint":"placeholder"}`))
	}))
	defer server.Close()
	docs := []schemaDocument{}
	plans := []schemaPlan{}
	for _, name := range []string{"first", "second", "third"} {
		docs = append(docs, schemaDocument{Target: name, Endpoint: "/channels/" + name, Body: map[string]any{"fields": []any{}}})
		plans = append(plans, schemaPlan{Fingerprint: "expected", Ops: []map[string]any{{"kind": "add_field", "risk": "safe"}}})
	}
	err := applySchemaDocuments(context.Background(), api.New(server.URL, "test"), docs, plans, true)
	if err == nil || !strings.Contains(err.Error(), "completed: [first]") || !strings.Contains(err.Error(), "unattempted: [third]") {
		t.Fatalf("unexpected error %v", err)
	}
	if !reflect.DeepEqual(requests, []string{"/channels/first/apply", "/channels/second/apply"}) {
		t.Fatalf("unexpected writes: %v", requests)
	}
}

func TestSchemaPlanReadonlyAndRenameWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels/test/plan" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fingerprint":"abc","ops":[{"kind":"add_field","field":"summary","risk":"safe"}],"drift":[{"field":"excerpt","type":"text"}]}`))
	}))
	defer server.Close()
	doc := schemaDocument{Target: "channel:test", Endpoint: "/channels/test", Body: map[string]any{"fields": []any{map[string]any{"name": "summary", "type": "text"}}}}
	plan, err := fetchSchemaPlan(context.Background(), api.New(server.URL, "test").WithReadonly(true), doc, false)
	if err != nil {
		t.Fatal(err)
	}
	if schemaPlanExit([]schemaPlan{plan}) != 1 || len(plan.Warnings) != 1 {
		t.Fatalf("rename not blocked: %#v", plan)
	}
}

func TestSchemaCyclesOnlyIncludeChannels(t *testing.T) {
	docs := []schemaDocument{
		{Target: "channel:a", Endpoint: "/channels/a", Body: map[string]any{"slug": "a", "fields": []any{map[string]any{"reference": "b"}}}},
		{Target: "channel:b", Endpoint: "/channels/b", Body: map[string]any{"slug": "b", "fields": []any{map[string]any{"reference": "a"}}}},
		{Target: "blog:a", Endpoint: "/blogs/a", Body: map[string]any{"slug": "a", "fields": []any{}}},
	}
	if got := schemaCycleTargets(docs); !reflect.DeepEqual(got, map[string]bool{"a": true, "b": true}) {
		t.Fatalf("cycles %v", got)
	}
	var applied []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/plan") {
			_, _ = w.Write([]byte(`{"fingerprint":"empty","exists":false,"ops":[{"kind":"create_target","risk":"safe"}]}`))
			return
		}
		applied = append(applied, r.URL.Path)
		_, _ = w.Write([]byte(`{"applied":true,"fingerprint":"placeholder"}`))
	}))
	defer server.Close()
	created, err := prepareSchemaCycles(context.Background(), api.New(server.URL, "test"), docs, make([]schemaPlan, 3))
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 || !reflect.DeepEqual(applied, []string{"/channels/a/apply", "/channels/b/apply"}) {
		t.Fatalf("created %v requests %v", created, applied)
	}
}

func TestSchemaLoadRejectsMultipleYAMLDocuments(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "schema"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "schema", "products.yml"), []byte("fields: []\n---\nfields: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSchemaDocuments(root); err == nil {
		t.Fatal("accepted multiple documents")
	}
}

func TestSchemaWriteRenamePreservesUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "products.yml")
	if err := os.WriteFile(path, []byte("unknown: keep\nfields:\n - name: summary\n   custom: keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeSchemaRename(path, "summary", "excerpt"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"unknown: keep", "custom: keep", "renamed_from: excerpt"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %q in %s", want, data)
		}
	}
}

func TestSchemaApplyCyclesPreservePlaceholderWithoutPrune(t *testing.T) {
	existing := map[string]bool{}
	applied := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		target := strings.TrimSuffix(strings.TrimSuffix(r.URL.Path, "/plan"), "/apply")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(r.URL.Path, "/plan") {
			if existing[target] {
				_, _ = w.Write([]byte(`{"fingerprint":"placeholder","exists":true,"ops":[{"kind":"add_field","field":"title","risk":"safe"}],"drift":[{"field":"nimbu_schema_placeholder","type":"string"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"fingerprint":"missing","exists":false,"ops":[{"kind":"create_target","risk":"safe"}]}`))
			}
			return
		}
		if body["prune"] != nil {
			t.Errorf("unexpected implicit prune: %v", body)
		}
		existing[target] = true
		applied = append(applied, target)
		_, _ = w.Write([]byte(`{"applied":true,"fingerprint":"placeholder"}`))
	}))
	defer server.Close()
	docs := []schemaDocument{}
	for _, pair := range [][2]string{{"a", "b"}, {"b", "a"}} {
		docs = append(docs, schemaDocument{Target: "channel:" + pair[0], Endpoint: "/channels/" + pair[0], Body: map[string]any{"slug": pair[0], "fields": []any{map[string]any{"name": "title", "type": "string"}, map[string]any{"name": "related", "type": "belongs_to", "reference": pair[1]}}}})
	}
	plans := []schemaPlan{{Fingerprint: "missing", Ops: []map[string]any{{"kind": "create_target", "risk": "safe"}}}, {Fingerprint: "missing", Ops: []map[string]any{{"kind": "create_target", "risk": "safe"}}}}
	if err := applySchemaDocuments(context.Background(), api.New(server.URL, "test"), docs, plans, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(applied, []string{"/channels/a", "/channels/b", "/channels/a", "/channels/b"}) {
		t.Fatalf("apply order %v", applied)
	}
}

func TestSchemaRefreshRetainsOriginalFingerprintGuard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/apply") {
			t.Error("applied stale plan")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fingerprint":"changed","exists":true,"ops":[{"kind":"remove_field","risk":"destructive"}]}`))
	}))
	defer server.Close()
	doc := schemaDocument{Target: "products", Endpoint: "/products/customizations", Body: map[string]any{"fields": []any{}}}
	err := applySchemaDocuments(context.Background(), api.New(server.URL, "test"), []schemaDocument{doc}, []schemaPlan{{Fingerprint: "approved", Warnings: []string{"reference 'pending' not found on target; apply it first"}}}, false)
	if err == nil || !strings.Contains(err.Error(), "changed since approval") {
		t.Fatalf("unexpected error %v", err)
	}
}
