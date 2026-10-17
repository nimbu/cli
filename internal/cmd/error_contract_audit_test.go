package cmd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// assertTyped fails unless err classifies as code with exit ExitUsage.
func assertTyped(t *testing.T, err error, code canonicalErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s, got nil", code)
	}
	if desc := classifyError(err); desc.Code != code || desc.ExitCode != ExitUsage {
		t.Fatalf("err %q = %s/%d, want %s/%d", err, desc.Code, desc.ExitCode, code, ExitUsage)
	}
}

func TestEnvironmentUsageErrorsAreTyped(t *testing.T) {
	t.Run("unknown environment", func(t *testing.T) {
		environmentProject(t)
		err := applyEnvironment(&RootFlags{Environment: "nope"}, nil)
		assertTyped(t, err, errorUsageInvalid)
		if desc := classifyError(err); !strings.Contains(desc.Hint, "nimbu env list") {
			t.Fatalf("hint = %q", desc.Hint)
		}
	})
	t.Run("--env outside a project", func(t *testing.T) {
		t.Setenv(config.ProjectDirEnv, t.TempDir())
		assertTyped(t, applyEnvironment(&RootFlags{Environment: "staging"}, nil), errorUsageInvalid)
	})
	t.Run("env add without --site", func(t *testing.T) {
		environmentProject(t)
		err := (&EnvAddCmd{Name: "qa", Host: "api.example.test"}).Run(context.Background(), &RootFlags{})
		assertTyped(t, err, errorSiteRequired)
	})
}

func TestSitesCurrentReportsUnreadableProject(t *testing.T) {
	original := readProjectSite
	readProjectSite = func() (string, error) { return "", errors.New("yaml: line 1: did not find expected ',' or ']'") }
	t.Cleanup(func() { readProjectSite = original })

	for _, mode := range []output.Mode{{}, {JSON: true}, {Plain: true}} {
		ctx, out, _ := newSitesListTestContext(t, "https://api.example.test", mode)
		ctx = context.WithValue(ctx, configKey{}, &config.Config{DefaultSite: "global-default"})
		err := (&SitesCurrentCmd{}).Run(ctx, &RootFlags{})
		assertTyped(t, err, errorSiteRequired)
		if !strings.Contains(err.Error(), "yaml: line 1") {
			t.Fatalf("error hides the cause: %v", err)
		}
		if out.Len() != 0 {
			t.Fatalf("mode %+v printed a result: %q", mode, out.String())
		}
	}
}

func TestThemesPushReportsErrorsBeforeTheTimeline(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "nimbu.yml"), []byte("site: demo\ntheme: default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "layouts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "layouts", "default.liquid"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The timeline label lookup only needs a fast answer.
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("NIMBU_TOKEN", "test-token")
	t.Setenv("NIMBU_JSON", "")
	t.Setenv("NIMBU_PLAIN", "")

	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"selector": {[]string{"--only", "nothing/here.liquid"}, "matched no managed files"},
		"build":    {[]string{"--build"}, "sync.build.command is not configured"},
	} {
		t.Run(name, func(t *testing.T) {
			var err error
			stderr := captureStderr(t, func() {
				err = withWorkingDir(t, root, func() error {
					return execute(append([]string{"--apiurl", server.URL, "themes", "push"}, tc.args...))
				})
			})
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != ExitUsage {
				t.Fatalf("want exit %d, got %v", ExitUsage, err)
			}
			if !strings.Contains(stderr, "error: ") || !strings.Contains(stderr, tc.want) {
				t.Fatalf("human mode must explain the failure, stderr = %q", stderr)
			}
		})
	}
}

func TestMenusCopyExistingMenuNeedsConfirmation(t *testing.T) {
	var (
		mu      sync.Mutex
		patched int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/menus":
			_, _ = w.Write([]byte(`[{"slug":"main","name":"Main"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/menus/main":
			_, _ = w.Write([]byte(`{"slug":"main","name":"Main"}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/menus/main":
			mu.Lock()
			patched++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"slug":"main","name":"Main"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("NIMBU_TOKEN", "test-token")
	args := []string{"--json", "menus", "copy", "--from", "a", "--to", "b", "--from-host", server.URL, "--to-host", server.URL}

	var err error
	stderr := captureStderr(t, func() {
		captureStdout(t, func() error { err = execute(args); return nil })
	})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitUsage {
		t.Fatalf("want exit %d, got %v", ExitUsage, err)
	}
	if envelope := decodeEnvelope(t, stderr); envelope.Error.Code != errorConfirmationRequired {
		t.Fatalf("code = %s", envelope.Error.Code)
	}
	if patched != 0 {
		t.Fatal("menu overwritten without confirmation")
	}

	// --yes confirms the overwrite, as it answers the single-menu prompt.
	captureStderr(t, func() {
		captureStdout(t, func() error { err = execute(append(args, "--yes")); return nil })
	})
	if err != nil || patched != 1 {
		t.Fatalf("--yes: err=%v patched=%d", err, patched)
	}
}
