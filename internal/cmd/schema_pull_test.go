package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nimbu/cli/internal/api"
)

func TestSchemaPullSelectedSnapshot(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/channels/events/schema":
			_, _ = w.Write([]byte(`{"slug":"events","name":"Events","publishable":true,"rss_enabled":false,"id":"secret","url":"https://example.test","fields":[{"name":"zebra","type":"select","position":3,"id":"f1","options":[{"name":"Zed","slug":"z","id":"o1","position":1},{"name":"Alpha","slug":"a"}]},{"name":"alpha","type":"string","required":false},{"name":"_status","type":"select"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	path := filepath.Join(root, "schema", "channels", "events.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("slug: events\nx-team: content\nfields:\n  - name: zebra\n    renamed_from: old_zebra\n    x-note: retain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	written, err := pullSchemas(context.Background(), api.New(server.URL, "token"), root, []string{"channels/events"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(written, []string{path}) {
		t.Fatalf("written: %v", written)
	}
	if !reflect.DeepEqual(paths, []string{"/channels/events/schema"}) {
		t.Fatalf("paths: %v", paths)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] != nil || got["url"] != nil || got["publishable"] != true || got["rss_enabled"] != false || got["x-team"] != "content" {
		t.Fatalf("document: %#v", got)
	}
	fields := got["fields"].([]any)
	if len(fields) != 2 {
		t.Fatalf("fields: %#v", fields)
	}
	first := fields[0].(map[string]any)
	if first["name"] != "zebra" || first["id"] != nil || first["position"] != nil || first["renamed_from"] != "old_zebra" {
		t.Fatalf("field: %#v", first)
	}
	opts := first["options"].([]any)
	if opts[0].(map[string]any)["slug"] != "z" || opts[0].(map[string]any)["id"] != nil {
		t.Fatalf("options: %#v", opts)
	}
}

func TestSchemaPullFetchFailurePreservesFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/customers/customizations/schema" {
			_, _ = w.Write([]byte(`{"fields":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	root := t.TempDir()
	path := filepath.Join(root, "schema", "customers.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("fields: []\nx-note: unchanged\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := pullSchemas(context.Background(), api.New(server.URL, "token"), root, []string{"customers", "products"})
	if err == nil {
		t.Fatal("expected missing products error")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Fatalf("file changed: %s", got)
	}
}

func TestSchemaPullListsCheckoutProfiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/checkout_profiles":
			_, _ = w.Write([]byte(`[{"slug":"gift"}]`))
		case "/products/checkout_profiles/gift/schema":
			_, _ = w.Write([]byte(`{"slug":"gift","name":"Gift","private_fields":[],"fields":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	paths, err := pullSchemas(context.Background(), api.New(server.URL, "token"), root, []string{"checkout_profiles"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != filepath.Join(root, "schema", "checkout_profiles", "gift.yml") {
		t.Fatalf("paths: %v", paths)
	}
}

func TestSchemaPullRejectsInvalidSelectors(t *testing.T) {
	for _, selector := range []string{"channels/..", "channels/", "channels/a/b", "products/anything", "unknown", "channels/a\\b"} {
		if validSchemaPullSelector(selector) {
			t.Errorf("accepted %q", selector)
		}
	}
}

func TestSchemaPullRejectsMalformedResponsesWithoutOverwriting(t *testing.T) {
	for _, response := range []string{
		`{"slug":"other","fields":[]}`,
		`{"slug":"events","fields":[null]}`,
		`{"slug":"events","fields":[{"type":"string"}]}`,
		`{"slug":"events","fields":[{"name":"title"},{"name":"title"}]}`,
		`{"slug":"events","fields":[{"name":"category","options":null}]}`,
		`{"slug":"events","fields":[{"name":"category","options":[null]}]}`,
		`{"slug":"events","fields":[{"name":"category","options":[{"slug":"a"},{"slug":"a"}]}]}`,
	} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()
			root := t.TempDir()
			path := filepath.Join(root, "schema", "channels", "events.yml")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			original := "slug: events\nfields: []\n"
			if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := pullSchemas(context.Background(), api.New(server.URL, "token"), root, []string{"channels/events"}); err == nil {
				t.Fatal("accepted malformed response")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != original {
				t.Fatalf("file changed: %s; %v", got, err)
			}
		})
	}
}

func TestSchemaPullRejectsMultipleLocalDocuments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"fields":[]}`))
	}))
	defer server.Close()
	root := t.TempDir()
	path := filepath.Join(root, "schema", "customers.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "fields: []\n---\nx-note: retained\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := pullSchemas(context.Background(), api.New(server.URL, "token"), root, []string{"customers"})
	if err == nil || !strings.Contains(err.Error(), "single YAML document") {
		t.Fatalf("error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != original {
		t.Fatalf("file changed: %s; %v", got, err)
	}
}
