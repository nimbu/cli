package themes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestRunDiffReportsChangedAndMissingLiquidFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "layouts"), 0o755); err != nil {
		t.Fatalf("mkdir layouts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "layouts", "default.liquid"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write layout: %v", err)
	}

	cfg := Config{
		ProjectRoot: root,
		Theme:       "demo",
		Roots: []RootSpec{
			{Kind: KindLayout, LocalPath: "layouts"},
			{Kind: KindSnippet, LocalPath: "snippets"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/themes/demo":
			_, _ = w.Write([]byte(`{
				"layouts":[{"name":"default.liquid"}],
				"snippets":[{"name":"header.liquid"}]
			}`))
		case "/themes/demo/layouts/default.liquid":
			_, _ = w.Write([]byte(`{"name":"default.liquid","code":"new"}`))
		case "/themes/demo/snippets/header.liquid":
			_, _ = w.Write([]byte(`{"name":"header.liquid","code":"header"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := api.New(srv.URL, "token")
	result, err := RunDiff(context.Background(), client, cfg)
	if err != nil {
		t.Fatalf("run diff: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entry count = %d", len(result.Entries))
	}
	if result.Entries[0].Status != "changed" || result.Entries[0].Path != "layouts/default.liquid" {
		t.Fatalf("unexpected first entry: %#v", result.Entries[0])
	}
	if result.Entries[1].Status != "missing" || result.Entries[1].Path != "snippets/header.liquid" {
		t.Fatalf("unexpected second entry: %#v", result.Entries[1])
	}
	for _, entry := range result.Entries {
		if entry.Diff != "" {
			t.Fatalf("expected no diff content without Content option, got %q", entry.Diff)
		}
	}
}

func TestRunDiffWithContentRendersUnifiedDiff(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "layouts"), 0o755); err != nil {
		t.Fatalf("mkdir layouts: %v", err)
	}
	local := "line one\nline changed\nline three\n"
	if err := os.WriteFile(filepath.Join(root, "layouts", "default.liquid"), []byte(local), 0o644); err != nil {
		t.Fatalf("write layout: %v", err)
	}

	cfg := Config{
		ProjectRoot: root,
		Theme:       "demo",
		Roots: []RootSpec{
			{Kind: KindLayout, LocalPath: "layouts"},
			{Kind: KindSnippet, LocalPath: "snippets"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/themes/demo":
			_, _ = w.Write([]byte(`{
				"layouts":[{"name":"default.liquid"}],
				"snippets":[{"name":"header.liquid"}]
			}`))
		case "/themes/demo/layouts/default.liquid":
			_, _ = w.Write([]byte(`{"name":"default.liquid","code":"line one\nline two\nline three\n"}`))
		case "/themes/demo/snippets/header.liquid":
			_, _ = w.Write([]byte(`{"name":"header.liquid","code":"header line\n"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := api.New(srv.URL, "token")
	result, err := RunDiffWithOptions(context.Background(), client, cfg, DiffOptions{Content: true})
	if err != nil {
		t.Fatalf("run diff: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entry count = %d", len(result.Entries))
	}

	changed := result.Entries[0]
	if changed.Status != "changed" || changed.Path != "layouts/default.liquid" {
		t.Fatalf("unexpected first entry: %#v", changed)
	}
	for _, want := range []string{
		"--- remote/layouts/default.liquid",
		"+++ local/layouts/default.liquid",
		"@@",
		"-line two",
		"+line changed",
	} {
		if !strings.Contains(changed.Diff, want) {
			t.Fatalf("changed diff missing %q:\n%s", want, changed.Diff)
		}
	}

	missing := result.Entries[1]
	if missing.Status != "missing" || missing.Path != "snippets/header.liquid" {
		t.Fatalf("unexpected second entry: %#v", missing)
	}
	for _, want := range []string{
		"--- remote/snippets/header.liquid",
		"+++ local/snippets/header.liquid",
		"-header line",
	} {
		if !strings.Contains(missing.Diff, want) {
			t.Fatalf("missing diff missing %q:\n%s", want, missing.Diff)
		}
	}
}

func TestUnifiedDiffNormalizesLineEndings(t *testing.T) {
	diff := unifiedDiff("templates/index.liquid", "alpha\r\nbeta\r\n", "alpha\ngamma")
	for _, want := range []string{"-beta", "+gamma"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("diff missing %q:\n%s", want, diff)
		}
	}
	if strings.Contains(diff, "\r") {
		t.Fatalf("diff should not contain carriage returns:\n%s", diff)
	}
	if strings.Contains(diff, "No newline") {
		t.Fatalf("diff should not report missing trailing newline:\n%s", diff)
	}
}
