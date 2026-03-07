package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestAppsPushRejectsSyncWithSubset(t *testing.T) {
	ctx := newAppsTestContext(t, "https://api.example.test")
	cmd := &AppsPushCmd{
		App:   "storefront",
		Sync:  true,
		Files: []string{"code/main.js"},
	}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatalf("expected sync/subset error, got %v", err)
	}
}

func TestAppsConfigWritesProjectAppEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apps":
			_, _ = w.Write([]byte(`[{"key":"storefront","name":"Storefront"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx := newAppsTestContext(t, srv.URL)
	withTempCWD(t, t.TempDir(), func() {
		withTempStdin(t, "1\n\n\n\n", func() {
			cmd := &AppsConfigCmd{}
			if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
				t.Fatalf("run apps config: %v", err)
			}
		})

		cfg, err := config.ReadProjectConfigFrom(filepath.Join(".", config.ProjectFileName))
		if err != nil {
			t.Fatalf("read project config: %v", err)
		}
		if len(cfg.Apps) != 1 {
			t.Fatalf("app count = %d", len(cfg.Apps))
		}
		if cfg.Apps[0].ID != "storefront" || cfg.Apps[0].Dir != "code" || cfg.Apps[0].Glob != "**/*.js" {
			t.Fatalf("unexpected app config: %#v", cfg.Apps[0])
		}
	})
}

func TestAppsConfigRejectsNoInput(t *testing.T) {
	ctx := newAppsTestContext(t, "https://api.example.test")
	cmd := &AppsConfigCmd{}
	err := cmd.Run(ctx, &RootFlags{Site: "demo", NoInput: true})
	if err == nil || !strings.Contains(err.Error(), "interactive only") {
		t.Fatalf("expected interactive-only error, got %v", err)
	}
}

func newAppsTestContext(t *testing.T, apiURL string) context.Context {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")

	flags := &RootFlags{APIURL: apiURL, Site: "demo"}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"

	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &strings.Builder{}, Err: &strings.Builder{}, Mode: output.Mode{}, NoTTY: true})
	return ctx
}

func withTempStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	orig := os.Stdin
	file, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("create temp stdin: %v", err)
	}
	if _, err := file.WriteString(input); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatalf("seek stdin: %v", err)
	}
	os.Stdin = file
	defer func() {
		os.Stdin = orig
		_ = file.Close()
	}()
	fn()
}
