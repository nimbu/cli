package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestThemesGetRunPrintsThemeInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/themes/storefront":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"theme":{"id":"theme-1","name":"Storefront","active":true,"created_at":"2024-01-02T03:04:05Z","updated_at":"2024-06-07T08:09:10Z"}}`))
		case "/themes/storefront/info":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cdn_base_path":"s/acme/themes/storefront/","cdn_host":"https://cdn.example.test","cdn_root":"https://cdn.example.test/s/acme/themes/storefront/","site_id":"site-1","site_short_id":"acme","theme_id":"theme-1","theme_short_id":"storefront"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	ctx, stdout, stderr := newThemesCDNRootTestContext(t, server.URL, output.Mode{})
	cmd := ThemesGetCmd{Theme: "storefront"}

	if err := withWorkingDir(t, writeThemeProjectConfig(t, "site: acme\ntheme: storefront\n"), func() error {
		return cmd.Run(ctx, &RootFlags{})
	}); err != nil {
		t.Fatalf("run command: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{
		"theme-1",
		"Storefront",
		"storefront",
		"CDN Root: https://cdn.example.test/s/acme/themes/storefront/",
		"CDN Host: https://cdn.example.test",
		"CDN Path: s/acme/themes/storefront/",
		"Active: true",
		"Site ID: site-1",
		"Site Short: acme",
		"Created: 2024-01-02 03:04:05",
		"Updated: 2024-06-07 08:09:10",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in output:\n%s", want, got)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestThemesGetRunJSONIncludesThemeInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/themes/storefront":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"theme":{"id":"theme-1","name":"Storefront"}}`))
		case "/themes/storefront/info":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cdn_base_path":"s/acme/themes/storefront/","cdn_host":"https://cdn.example.test","cdn_root":"https://cdn.example.test/s/acme/themes/storefront/","site_id":"site-1","site_short_id":"acme","theme_short_id":"storefront"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	ctx, stdout, _ := newThemesCDNRootTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ThemesGetCmd{Theme: "storefront"}

	if err := withWorkingDir(t, writeThemeProjectConfig(t, "site: acme\ntheme: storefront\n"), func() error {
		return cmd.Run(ctx, &RootFlags{})
	}); err != nil {
		t.Fatalf("run command: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	for key, want := range map[string]string{
		"id":             "theme-1",
		"name":           "Storefront",
		"cdn_base_path":  "s/acme/themes/storefront/",
		"cdn_host":       "https://cdn.example.test",
		"cdn_root":       "https://cdn.example.test/s/acme/themes/storefront/",
		"site_id":        "site-1",
		"site_short_id":  "acme",
		"theme_short_id": "storefront",
	} {
		if got[key] != want {
			t.Fatalf("expected %s=%q, got %#v", key, want, got[key])
		}
	}
}
