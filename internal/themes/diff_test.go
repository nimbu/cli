package themes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
}
