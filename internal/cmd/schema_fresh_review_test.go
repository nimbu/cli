package cmd

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
)

func TestSchemaUnresolvedReferenceCannotSucceedAsNoOp(t *testing.T) {
	plan := schemaPlan{Fingerprint: "current", Exists: true, Warnings: []string{"reference 'missing' not found on target; apply it first"}}
	if schemaPlanExit([]schemaPlan{plan}) == 0 {
		t.Error("unresolved reference reported no pending changes")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products/customizations/plan" {
			t.Errorf("unexpected write %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(plan)
	}))
	defer server.Close()
	doc := schemaDocument{Target: "products", Endpoint: "/products/customizations", Body: map[string]any{"fields": []any{}}}

	err := applySchemaDocuments(context.Background(), api.New(server.URL, "test"), []schemaDocument{doc}, []schemaPlan{plan}, false)

	if err == nil || !strings.Contains(err.Error(), "unresolved reference") {
		t.Fatalf("expected unresolved-reference failure, got %v", err)
	}
}

func TestSchemaCycleRejectsEditsAfterPlaceholderCreation(t *testing.T) {
	placeholders := 0
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fields, _ := body["fields"].([]any)
		field, _ := fields[0].(map[string]any)
		stub := field["name"] == "nimbu_schema_placeholder"
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/plan") {
			if stub {
				_, _ = w.Write([]byte(`{"fingerprint":"missing","exists":false,"ops":[{"kind":"create_target","risk":"safe"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"fingerprint":"concurrent-edit","exists":true,"ops":[{"kind":"remove_field","risk":"destructive"}]}`))
			}
			return
		}
		if stub {
			placeholders++
		} else {
			writes++
		}
		_, _ = w.Write([]byte(`{"applied":true,"fingerprint":"created-placeholder"}`))
	}))
	defer server.Close()
	var docs []schemaDocument
	var plans []schemaPlan
	for _, pair := range [][2]string{{"a", "b"}, {"b", "a"}} {
		docs = append(docs, schemaDocument{Target: "channel:" + pair[0], Endpoint: "/channels/" + pair[0], Body: map[string]any{"slug": pair[0], "fields": []any{map[string]any{"name": "related", "type": "belongs_to", "reference": pair[1]}}}})
		plans = append(plans, schemaPlan{Fingerprint: "missing", Ops: []map[string]any{{"kind": "create_target", "risk": "safe"}}})
	}

	err := applySchemaDocuments(context.Background(), api.New(server.URL, "test"), docs, plans, true)

	if err == nil || !strings.Contains(err.Error(), "changed since") || writes != 0 || placeholders != 2 {
		t.Fatalf("err=%v, real writes=%d, placeholders=%d", err, writes, placeholders)
	}
}

func TestSchemaRenameRejectsChangedFileWithoutWriting(t *testing.T) {
	for _, content := range []string{"", "[]\n", "fields: []\n", "fields:\n - name: summary\n   renamed_from: other\n", "fields:\n - name: summary\n---\nfields: []\n"} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "products.yml")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := writeSchemaRename(path, "summary", "excerpt"); err == nil {
				t.Fatal("accepted a changed or invalid rename document")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != content {
				t.Fatal("modified rejected document")
			}
		})
	}
}

func TestSchemaBuiltinReferencesDoNotCreateChannelDependencies(t *testing.T) {
	for _, builtin := range []string{"products", "customers", "orders", "pages", "articles"} {
		t.Run(builtin, func(t *testing.T) {
			docs := []schemaDocument{
				{Target: "channel:events", Endpoint: "/channels/events", Body: map[string]any{"slug": "events", "fields": []any{map[string]any{"reference": builtin}}}},
				{Target: "channel:" + builtin, Endpoint: "/channels/" + builtin, Body: map[string]any{"slug": builtin, "fields": []any{map[string]any{"reference": "events"}}}},
			}
			if got := schemaCycleTargets(docs); len(got) != 0 {
				t.Fatalf("builtin reference created false cycle: %v", got)
			}
			if got := orderSchemaDocuments(docs); got[0].Target != "channel:events" {
				t.Fatalf("builtin reference reordered channels: %v", got)
			}
		})
	}
}
