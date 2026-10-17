package cmd

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// themeConflictServer answers a release with one theme stage: every theme
// upload without force=true conflicts, like a file a teammate edited remotely.
func themeConflictServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var (
		mu     sync.Mutex
		forced []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/themes/"):
			if r.URL.Query().Get("force") == "true" {
				mu.Lock()
				forced = append(forced, r.URL.Path)
				mu.Unlock()
				// Echo the upload so the post-upload verification matches.
				_, _ = io.Copy(w, r.Body)
				return
			}
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"Conflict (a teammate edited theme.liquid)"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/releases":
			_, _ = w.Write([]byte(`{"id":"release-id"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/releases":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/themes/"):
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), forced...)
	}
}

func releaseThemeFixture(t *testing.T) string {
	t.Helper()
	root := releaseProjectFixture(t)
	if err := os.WriteFile(filepath.Join(root, "nimbu.yml"), []byte("site: demo\ntheme: default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "layouts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "layouts", "theme.liquid"), []byte("{{ content_for_layout }}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestReleaseYesDoesNotForceThemeConflicts(t *testing.T) {
	server, forced := themeConflictServer(t)
	root := releaseThemeFixture(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("NIMBU_TOKEN", "test-token")

	err := withWorkingDir(t, root, func() error {
		return execute([]string{"--apiurl", server.URL, "release", "--only", "theme", "--allow-dirty", "--yes", "--no-input"})
	})
	if got := forced(); len(got) != 0 {
		t.Fatalf("--yes force-overwrote remote theme files: %v", got)
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitUsage {
		t.Fatalf("want exit %d (confirmation required), got %v", ExitUsage, err)
	}
}

func TestReleaseYesForceOverwritesThemeConflicts(t *testing.T) {
	server, forced := themeConflictServer(t)
	root := releaseThemeFixture(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("NIMBU_TOKEN", "test-token")

	err := withWorkingDir(t, root, func() error {
		return execute([]string{"--apiurl", server.URL, "release", "--only", "theme", "--allow-dirty", "--yes", "--force", "--no-input"})
	})
	if err != nil {
		t.Fatalf("release --yes --force: %v", err)
	}
	if got := forced(); len(got) == 0 {
		t.Fatal("--force did not overwrite the conflicting theme file")
	}
}
