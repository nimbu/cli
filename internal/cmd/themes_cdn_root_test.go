package cmd

import (
	"bytes"
	"context"
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

func TestThemesCDNRootRunPlain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/themes/storefront/info" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Nimbu-Site"); got != "acme" {
			t.Fatalf("unexpected site header: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cdn_root":"https://cdn.example.test/s/acme/themes/storefront/"}`))
	}))
	defer server.Close()

	ctx, stdout, stderr := newThemesCDNRootTestContext(t, server.URL, output.Mode{Plain: true})
	cmd := ThemesCDNRootCmd{}

	if err := withWorkingDir(t, writeThemeProjectConfig(t, "site: acme\ntheme: storefront\n"), func() error {
		return cmd.Run(ctx, &RootFlags{})
	}); err != nil {
		t.Fatalf("run command: %v", err)
	}

	if got := strings.TrimSpace(stdout.String()); got != "https://cdn.example.test/s/acme/themes/storefront/" {
		t.Fatalf("unexpected stdout: %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestThemesCDNRootRunJSONUsesOverrideTheme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/themes/preview/info" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cdn_root":"https://cdn.example.test/s/acme/themes/preview/"}`))
	}))
	defer server.Close()

	ctx, stdout, _ := newThemesCDNRootTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ThemesCDNRootCmd{Theme: "preview"}

	if err := withWorkingDir(t, writeThemeProjectConfig(t, "site: acme\ntheme: storefront\n"), func() error {
		return cmd.Run(ctx, &RootFlags{})
	}); err != nil {
		t.Fatalf("run command: %v", err)
	}

	var got themeCDNRootPayload
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal json output: %v", err)
	}
	if got.Site != "acme" || got.Theme != "preview" || got.CDNRoot != "https://cdn.example.test/s/acme/themes/preview/" {
		t.Fatalf("unexpected json output: %+v", got)
	}
}

func TestThemesCDNRootRunErrorsWhenThemeCDNRootMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cdn_root":""}`))
	}))
	defer server.Close()

	ctx, _, _ := newThemesCDNRootTestContext(t, server.URL, output.Mode{Plain: true})
	cmd := ThemesCDNRootCmd{}

	err := withWorkingDir(t, writeThemeProjectConfig(t, "site: acme\ntheme: storefront\n"), func() error {
		return cmd.Run(ctx, &RootFlags{})
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `info returned empty cdn_root`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newThemesCDNRootTestContext(t *testing.T, apiURL string, mode output.Mode) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ctx := context.Background()
	ctx = output.WithMode(ctx, mode)
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   &stdout,
		Err:   &stderr,
		Mode:  mode,
		Color: "never",
		NoTTY: true,
	})
	ctx = context.WithValue(ctx, rootFlagsKey{}, &RootFlags{
		APIURL:  apiURL,
		Site:    "acme",
		Timeout: 2 * time.Second,
	})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})
	ctx = context.WithValue(ctx, authResolverKey{}, newAuthCredentialResolver("api.example.test"))

	if err := os.Setenv("NIMBU_TOKEN", "test-token"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("NIMBU_TOKEN")
	})

	return ctx, &stdout, &stderr
}

func writeThemeProjectConfig(t *testing.T, body string) string {
	t.Helper()

	root := t.TempDir()
	path := filepath.Join(root, config.ProjectFileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	return root
}

func withWorkingDir(t *testing.T, dir string, fn func() error) error {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if chdirErr := os.Chdir(cwd); chdirErr != nil {
			t.Fatalf("restore cwd: %v", chdirErr)
		}
	}()

	return fn()
}
