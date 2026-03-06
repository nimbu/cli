package themesync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestUpsertBytesAssetUsesSourcePayload(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/themes/demo/assets" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := api.New(server.URL, "token").WithSite("site")
	err := UpsertBytes(context.Background(), client, "demo", Resource{
		Kind:       KindAsset,
		RemoteName: "fonts/app.woff2",
	}, []byte("font-bytes"), true)
	if err != nil {
		t.Fatalf("upsert bytes: %v", err)
	}

	if captured["name"] != "fonts/app.woff2" {
		t.Fatalf("name mismatch: %#v", captured["name"])
	}
	source, ok := captured["source"].(map[string]any)
	if !ok {
		t.Fatalf("missing source payload: %#v", captured)
	}
	if source["__type"] != "File" {
		t.Fatalf("source type mismatch: %#v", source["__type"])
	}
	if source["filename"] != "app.woff2" {
		t.Fatalf("filename mismatch: %#v", source["filename"])
	}
}

func TestUpsertBytesTemplateUsesCodePayload(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/themes/demo/templates" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := api.New(server.URL, "token")
	err := UpsertBytes(context.Background(), client, "demo", Resource{
		Kind:       KindTemplate,
		RemoteName: "customers/login.liquid",
	}, []byte("{{ content }}"), false)
	if err != nil {
		t.Fatalf("upsert bytes: %v", err)
	}

	if captured["name"] != "customers/login.liquid" {
		t.Fatalf("name mismatch: %#v", captured["name"])
	}
	if captured["code"] != "{{ content }}" {
		t.Fatalf("code mismatch: %#v", captured["code"])
	}
	if _, ok := captured["source"]; ok {
		t.Fatalf("unexpected source payload: %#v", captured)
	}
}
