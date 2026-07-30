package cmd

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestAPIParsesVerbFirstAndLegacySyntax(t *testing.T) {
	tests := [][]string{
		{"api", "get", "/channels"},
		{"api", "GET", "/channels"},
		{"api", "patch", "/settings/shipping", "--data", `{"enabled":true}`},
		{"api", "--method", "GET", "--path", "/channels"},
	}

	for _, args := range tests {
		parser, _, err := newParser()
		if err != nil {
			t.Fatalf("new parser: %v", err)
		}
		if _, err := parser.Parse(args); err != nil {
			t.Errorf("parse %v: %v", args, err)
		}
	}
}

func TestRawAPIDataReadsAtFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(path, []byte(`{"enabled":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := rawAPIRequestBody(APIRequestFlags{Data: "@" + path})
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if body.(map[string]any)["enabled"] != true {
		t.Fatalf("body = %#v", body)
	}
}

func TestRawAPINonJSONOutputAddsNoNewline(t *testing.T) {
	var stdout bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: &stdout, Err: &bytes.Buffer{}})
	if err := outputRawAPIResponse(ctx, strings.NewReader("binary-ish")); err != nil {
		t.Fatalf("output response: %v", err)
	}
	if stdout.String() != "binary-ish" {
		t.Fatalf("output = %q", stdout.String())
	}
}

func TestRawAPIDeleteRequiresForce(t *testing.T) {
	err := runRawAPI(context.Background(), &RootFlags{}, http.MethodDelete, "/products/p1", APIRequestFlags{})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force error, got %v", err)
	}
}

func TestRawAPIRejectsAllWithBinaryOutput(t *testing.T) {
	err := runRawAPI(context.Background(), &RootFlags{}, http.MethodGet, "/uploads", APIRequestFlags{
		All: true, Output: "uploads.json",
	})
	if err == nil || !strings.Contains(err.Error(), "--all") || !strings.Contains(err.Error(), "--output") {
		t.Fatalf("expected incompatible flags error, got %v", err)
	}
}

func TestDownloadDoesNotOverwriteWithoutForce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := writeDownloadResponse(context.Background(), strings.NewReader("new"), path, false)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected overwrite error, got %v", err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != "old" {
		t.Fatalf("existing content changed to %q", content)
	}
}

func TestDownloadToStdoutUsesContextWriter(t *testing.T) {
	var stdout bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{
		Out:   &stdout,
		Err:   &bytes.Buffer{},
		NoTTY: true,
	})

	if err := writeDownloadResponse(ctx, strings.NewReader("exact\nbytes"), "-", false); err != nil {
		t.Fatalf("write download: %v", err)
	}
	if stdout.String() != "exact\nbytes" {
		t.Fatalf("output = %q", stdout.String())
	}
}
