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

func TestRemoteInManagedScope(t *testing.T) {
	cfg := Config{
		Roots: []RootSpec{
			{Kind: KindLayout, LocalPath: "layouts"},
			{Kind: KindAsset, LocalPath: "images", RemoteBase: "images"},
			{Kind: KindAsset, LocalPath: "fonts", RemoteBase: "fonts"},
		},
	}

	if !remoteInManagedScope(cfg, Resource{Kind: KindLayout, RemoteName: "default.liquid"}) {
		t.Fatal("expected layout in scope")
	}
	if !remoteInManagedScope(cfg, Resource{Kind: KindAsset, RemoteName: "fonts/app.woff2"}) {
		t.Fatal("expected font asset in scope")
	}
	if remoteInManagedScope(cfg, Resource{Kind: KindAsset, RemoteName: "javascripts/app.js"}) {
		t.Fatal("unexpected asset scope match")
	}
}

func TestScopeUsesAllFiles(t *testing.T) {
	tests := []struct {
		name        string
		opts        Options
		hasCategory bool
		want        bool
	}{
		{name: "default no flags", opts: Options{}, hasCategory: false, want: false},
		{name: "all flag", opts: Options{All: true}, hasCategory: false, want: true},
		{name: "only flag", opts: Options{Only: []string{"templates/page.liquid"}}, hasCategory: false, want: true},
		{name: "positional selector", opts: Options{Selectors: []string{"templates/page.liquid"}}, hasCategory: false, want: true},
		{name: "category alone", opts: Options{}, hasCategory: true, want: true},
		{name: "category with since", opts: Options{Since: "origin/main"}, hasCategory: true, want: false},
		{name: "only with since", opts: Options{Only: []string{"templates/page.liquid"}, Since: "origin/main"}, hasCategory: false, want: true},
		{name: "since alone", opts: Options{Since: "origin/main"}, hasCategory: false, want: false},
		{name: "all with since", opts: Options{All: true, Since: "origin/main"}, hasCategory: false, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scopeUsesAllFiles(tt.opts, tt.hasCategory)
			if got != tt.want {
				t.Fatalf("scopeUsesAllFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunPushPromptsAndRetriesConflictWithForce(t *testing.T) {
	root := t.TempDir()
	writeThemeTestFile(t, root, "templates/article.liquid", "local")

	var postQueries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/themes/demo/templates" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		postQueries = append(postQueries, r.URL.RawQuery)
		if len(postQueries) == 1 {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"Conflict (Peter edited article.liquid)"}`))
			return
		}
		if r.URL.Query().Get("force") != "true" {
			t.Fatalf("expected forced retry, got query %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	var prompted bool
	result, err := RunPush(context.Background(), api.New(server.URL, ""), themeTestConfig(root), Options{
		All: true,
		ConfirmOverwrite: func(_ context.Context, resource Resource, err error) (bool, error) {
			prompted = true
			if resource.DisplayPath != "templates/article.liquid" {
				t.Fatalf("unexpected prompt resource: %s", resource.DisplayPath)
			}
			return true, nil
		},
	})
	if err != nil {
		t.Fatalf("RunPush: %v", err)
	}
	if !prompted {
		t.Fatal("expected overwrite prompt")
	}
	if len(postQueries) != 2 || postQueries[0] != "" || postQueries[1] != "force=true" {
		t.Fatalf("unexpected post queries: %#v", postQueries)
	}
	if len(result.Uploaded) != 1 || result.Uploaded[0].DisplayPath != "templates/article.liquid" {
		t.Fatalf("unexpected uploads: %#v", result.Uploaded)
	}
}

func TestRunPushSkipsConflictWhenOverwriteDeclined(t *testing.T) {
	root := t.TempDir()
	writeThemeTestFile(t, root, "templates/article.liquid", "local")

	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"Conflict (Peter edited article.liquid)"}`))
	}))
	defer server.Close()

	result, err := RunPush(context.Background(), api.New(server.URL, ""), themeTestConfig(root), Options{
		All: true,
		ConfirmOverwrite: func(context.Context, Resource, error) (bool, error) {
			return false, nil
		},
	})
	if err != nil {
		t.Fatalf("RunPush: %v", err)
	}
	if posts != 1 {
		t.Fatalf("expected no forced retry, got %d posts", posts)
	}
	if len(result.Uploaded) != 0 {
		t.Fatalf("expected skipped upload to be omitted, got %#v", result.Uploaded)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].DisplayPath != "templates/article.liquid" {
		t.Fatalf("unexpected skipped uploads: %#v", result.Skipped)
	}
}

func TestRunPushForceBypassesConflictPrompt(t *testing.T) {
	root := t.TempDir()
	writeThemeTestFile(t, root, "templates/article.liquid", "local")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/themes/demo/templates" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("force") != "true" {
			t.Fatalf("expected initial force query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	result, err := RunPush(context.Background(), api.New(server.URL, ""), themeTestConfig(root), Options{
		All:   true,
		Force: true,
		ConfirmOverwrite: func(context.Context, Resource, error) (bool, error) {
			t.Fatal("unexpected overwrite prompt")
			return false, nil
		},
	})
	if err != nil {
		t.Fatalf("RunPush: %v", err)
	}
	if len(result.Uploaded) != 1 || len(result.Skipped) != 0 {
		t.Fatalf("unexpected result: uploaded=%#v skipped=%#v", result.Uploaded, result.Skipped)
	}
}

func themeTestConfig(root string) Config {
	return Config{
		ProjectRoot: root,
		Theme:       "demo",
		Roots: []RootSpec{{
			AbsPath:   filepath.Join(root, "templates"),
			Kind:      KindTemplate,
			LocalPath: "templates",
		}},
	}
}

func writeThemeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
