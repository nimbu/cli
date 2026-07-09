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
	"strings"
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

	cmd := &UploadsCreateCmd{Source: path}
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

func TestUploadsCreateFileFlagPostsSourceAttachmentJSON(t *testing.T) {
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
	if source["attachment"] != base64.StdEncoding.EncodeToString([]byte("<svg></svg>")) {
		t.Fatalf("attachment = %#v", source["attachment"])
	}
}

func TestUploadsCreateFileRefPostsSourceFileRefJSON(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "test-token")

	fileRef := "nimbu://archive/uploads/507f1f77bcf86cd799439014"

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
		_, _ = w.Write([]byte(`{"id":"copied","name":"manual.pdf","url":"https://cdn.target.test/manual.pdf","size":1234}`))
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

	cmd := &UploadsCreateCmd{FileRef: fileRef}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("run uploads create: %v", err)
	}

	source, ok := captured["source"].(map[string]any)
	if !ok {
		t.Fatalf("missing source payload: %#v", captured)
	}
	if source["__type"] != "FileRef" {
		t.Fatalf("source type = %#v", source["__type"])
	}
	if source["source"] != fileRef {
		t.Fatalf("source ref = %#v", source["source"])
	}
	if _, ok := source["attachment"]; ok {
		t.Fatalf("FileRef payload should not include attachment: %#v", source)
	}
}

func TestUploadsCreateRejectsNameWithFileRef(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "test-token")

	var out bytes.Buffer
	var errOut bytes.Buffer
	flags := &RootFlags{APIURL: "http://127.0.0.1:0", Site: "demo", Timeout: 2 * time.Second}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"
	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, output.Mode{JSON: true})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &out, Err: &errOut, Mode: output.Mode{JSON: true}, NoTTY: true})
	ctx = output.WithProgress(ctx, output.NewDisabledProgress())

	cmd := &UploadsCreateCmd{FileRef: "nimbu://archive/uploads/507f1f77bcf86cd799439014", Name: "renamed.pdf"}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Fatalf("expected error when --name is used with --file-ref")
	}
	if !strings.Contains(err.Error(), "--name cannot be used with --file-ref") {
		t.Fatalf("error = %v, want mention of --name/--file-ref incompatibility", err)
	}
}

func TestUploadsCreateRequiresSourceOrFile(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "test-token")

	var out bytes.Buffer
	var errOut bytes.Buffer
	flags := &RootFlags{APIURL: "http://127.0.0.1:0", Site: "demo", Timeout: 2 * time.Second}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"
	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, output.Mode{JSON: true})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &out, Err: &errOut, Mode: output.Mode{JSON: true}, NoTTY: true})
	ctx = output.WithProgress(ctx, output.NewDisabledProgress())

	cmd := &UploadsCreateCmd{}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Fatalf("expected error when neither --source nor --file is set")
	}
	if !strings.Contains(err.Error(), "--source, --file, or --file-ref") {
		t.Fatalf("error = %v, want mention of --source/--file/--file-ref", err)
	}
}

func TestUploadsCreateRejectsSourceAndFile(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "test-token")

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.svg")
	filePath := filepath.Join(dir, "file.svg")
	if err := os.WriteFile(sourcePath, []byte("<svg>source</svg>"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("<svg>file</svg>"), 0o644); err != nil {
		t.Fatalf("write file flag file: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	flags := &RootFlags{APIURL: "http://127.0.0.1:0", Site: "demo", Timeout: 2 * time.Second}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"
	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, output.Mode{JSON: true})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &out, Err: &errOut, Mode: output.Mode{JSON: true}, NoTTY: true})
	ctx = output.WithProgress(ctx, output.NewDisabledProgress())

	cmd := &UploadsCreateCmd{Source: sourcePath, File: filePath}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Fatalf("expected error when both --source and --file are set")
	}
	if !strings.Contains(err.Error(), "use only one of --source, --file, or --file-ref") {
		t.Fatalf("error = %v, want mention of source selector XOR", err)
	}
}
