package apps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/config"
)

func TestPullWritesRemoteCodeFilesUnderConfiguredDir(t *testing.T) {
	root := t.TempDir()
	app := AppConfig{
		AppProjectConfig: config.AppProjectConfig{ID: "storefront", Name: "storefront", Dir: "code", Glob: "**/*.js"},
		ProjectRoot:      root,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/apps/storefront/code" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[
			{"name":"main.js","code":"require('./lib/helper')\n"},
			{"name":"lib/helper.js","code":"module.exports = 1\n"}
		]`))
	}))
	defer srv.Close()

	result, err := Pull(context.Background(), api.New(srv.URL, "token"), app, PullOptions{})
	if err != nil {
		t.Fatalf("pull app code: %v", err)
	}

	if len(result.Written) != 2 {
		t.Fatalf("written count = %d, want 2: %#v", len(result.Written), result.Written)
	}
	assertFileContent(t, filepath.Join(root, "code", "main.js"), "require('./lib/helper')\n")
	assertFileContent(t, filepath.Join(root, "code", "lib", "helper.js"), "module.exports = 1\n")
}

func TestPullOnlyAcceptsRemoteAndProjectRelativeNames(t *testing.T) {
	root := t.TempDir()
	app := AppConfig{
		AppProjectConfig: config.AppProjectConfig{ID: "storefront", Name: "storefront", Dir: "code", Glob: "**/*.js"},
		ProjectRoot:      root,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/apps/storefront/code" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[
			{"name":"main.js","code":"main\n"},
			{"name":"lib/helper.js","code":"helper\n"}
		]`))
	}))
	defer srv.Close()

	result, err := Pull(context.Background(), api.New(srv.URL, "token"), app, PullOptions{Only: []string{"code/main.js", "lib/helper.js"}})
	if err != nil {
		t.Fatalf("pull app code: %v", err)
	}

	if got := strings.Join(result.Files, ","); got != "lib/helper.js,main.js" {
		t.Fatalf("files = %q, want lib/helper.js,main.js", got)
	}
}

func TestPullRejectsUnsafeRemoteNames(t *testing.T) {
	for _, remoteName := range []string{"nested/../secret.js", " /secret.js", "\\secret.js"} {
		t.Run(remoteName, func(t *testing.T) {
			root := t.TempDir()
			app := AppConfig{
				AppProjectConfig: config.AppProjectConfig{ID: "storefront", Name: "storefront", Dir: "code", Glob: "**/*.js"},
				ProjectRoot:      root,
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/apps/storefront/code" {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write([]byte(`[{"name":` + quoteJSON(remoteName) + `,"code":"leak\n"}]`))
			}))
			defer srv.Close()

			_, err := Pull(context.Background(), api.New(srv.URL, "token"), app, PullOptions{})
			if err == nil || !strings.Contains(err.Error(), "unsafe app code file name") {
				t.Fatalf("expected unsafe filename error, got %v", err)
			}
			if _, statErr := os.Stat(filepath.Join(root, "secret.js")); !os.IsNotExist(statErr) {
				t.Fatalf("unsafe file was written or unexpected stat error: %v", statErr)
			}
			if _, statErr := os.Stat(filepath.Join(root, "code", "secret.js")); !os.IsNotExist(statErr) {
				t.Fatalf("unsafe file was written under app dir or unexpected stat error: %v", statErr)
			}
		})
	}
}

func TestPullRejectsUnsafeConfiguredDir(t *testing.T) {
	root := t.TempDir()
	app := AppConfig{
		AppProjectConfig: config.AppProjectConfig{ID: "storefront", Name: "storefront", Dir: "../outside", Glob: "**/*.js"},
		ProjectRoot:      root,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/apps/storefront/code" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[{"name":"main.js","code":"leak\n"}]`))
	}))
	defer srv.Close()

	_, err := Pull(context.Background(), api.New(srv.URL, "token"), app, PullOptions{})
	if err == nil || !strings.Contains(err.Error(), "unsafe app code local path") {
		t.Fatalf("expected unsafe local path error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(root), "outside", "main.js")); !os.IsNotExist(statErr) {
		t.Fatalf("unsafe file was written outside project or unexpected stat error: %v", statErr)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, string(data), want)
	}
}

func quoteJSON(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
