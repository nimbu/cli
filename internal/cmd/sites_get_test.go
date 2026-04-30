package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func newSitesGetTestContext(t *testing.T, apiURL string, mode output.Mode) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")

	var stdout, stderr bytes.Buffer
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
		Site:    "demo",
		Timeout: 2 * time.Second,
	})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})
	return ctx, &stdout, &stderr
}

func TestSitesGetRunJSONIncludesComputedURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sites/demo" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"site-1","subdomain":"demo","name":"Demo Site"}`))
	}))
	defer server.Close()

	ctx, stdout, _ := newSitesGetTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := SitesGetCmd{}

	if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL, Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, stdout.String())
	}

	// admin_url should always use subdomain-based host
	if got["admin_url"] == nil {
		t.Fatal("missing admin_url in JSON output")
	}
	adminURL, _ := got["admin_url"].(string)
	if !strings.HasSuffix(adminURL, "/admin") {
		t.Fatalf("admin_url should end with /admin, got %q", adminURL)
	}
	if !strings.Contains(adminURL, "demo") {
		t.Fatalf("admin_url should contain subdomain, got %q", adminURL)
	}

	// live_url should fall back to subdomain host when no custom domain
	if got["live_url"] == nil {
		t.Fatal("missing live_url in JSON output")
	}
	liveURL, _ := got["live_url"].(string)
	if !strings.Contains(liveURL, "demo") {
		t.Fatalf("live_url should contain subdomain, got %q", liveURL)
	}
}

func TestSitesGetRunJSONCustomDomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"site-2","subdomain":"acme","name":"Acme","domain":"www.acme.com"}`))
	}))
	defer server.Close()

	ctx, stdout, _ := newSitesGetTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := SitesGetCmd{}

	if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL, Site: "acme"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// live_url should use the custom domain
	liveURL, _ := got["live_url"].(string)
	if liveURL != "https://www.acme.com" {
		t.Fatalf("expected live_url=https://www.acme.com, got %q", liveURL)
	}

	// admin_url should still use subdomain host, not custom domain
	adminURL, _ := got["admin_url"].(string)
	if strings.Contains(adminURL, "acme.com") {
		t.Fatalf("admin_url should not use custom domain, got %q", adminURL)
	}
	if !strings.Contains(adminURL, "acme") && !strings.HasSuffix(adminURL, "/admin") {
		t.Fatalf("admin_url unexpected: %q", adminURL)
	}
}

func TestSitesGetRunHumanShowsURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"site-1","subdomain":"demo","name":"Demo"}`))
	}))
	defer server.Close()

	ctx, stdout, _ := newSitesGetTestContext(t, server.URL, output.Mode{})
	cmd := SitesGetCmd{}

	if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL, Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{"Admin URL:", "Live URL:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestSitesGetRunStandardAPIURL(t *testing.T) {
	// Verify URL computation with the real nimbu.io API host
	host := siteHostFromAPI("https://api.nimbu.io", "sociopolis")
	if host != "sociopolis.nimbu.io" {
		t.Fatalf("expected sociopolis.nimbu.io, got %q", host)
	}

	liveURL := displaySiteURL(host)
	if liveURL != "https://sociopolis.nimbu.io" {
		t.Fatalf("expected https://sociopolis.nimbu.io, got %q", liveURL)
	}

	adminURL := strings.TrimRight(liveURL, "/") + "/admin"
	if adminURL != "https://sociopolis.nimbu.io/admin" {
		t.Fatalf("expected https://sociopolis.nimbu.io/admin, got %q", adminURL)
	}
}
