package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestUploadsCreatePostsSourceAttachmentJSON(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "test-token")

	dir := t.TempDir()
	path := filepath.Join(dir, "plus-regular.svg")
	if err := os.WriteFile(path, []byte("<svg></svg>"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/uploads" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"uploaded","name":"plus-regular.svg","url":"https://cdn.target.test/plus-regular.svg","size":11}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	flags := &RootFlags{APIURL: srv.URL, Site: "demo", Timeout: 2 * time.Second}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"
	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, output.Mode{JSON: true})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &out, Err: &errOut, Mode: output.Mode{JSON: true}, NoTTY: true})
	ctx = output.WithProgress(ctx, output.NewDisabledProgress())

	cmd := &UploadsCreateCmd{File: path}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("run uploads create: %v", err)
	}

	source, ok := captured["source"].(map[string]any)
	if !ok {
		t.Fatalf("missing source payload: %#v", captured)
	}
	if source["__type"] != "File" {
		t.Fatalf("source type = %#v", source["__type"])
	}
	if source["filename"] != "plus-regular.svg" {
		t.Fatalf("filename = %#v", source["filename"])
	}
	if source["content_type"] != "image/svg+xml" {
		t.Fatalf("content type = %#v", source["content_type"])
	}
	if source["attachment"] != base64.StdEncoding.EncodeToString([]byte("<svg></svg>")) {
		t.Fatalf("attachment = %#v", source["attachment"])
	}
}
