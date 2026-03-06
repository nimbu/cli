package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

func TestServerPresenterPrintSummary(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	presenter := newServerPresenter(ctx, false)
	presenter.PrintBanner()
	presenter.PrintSummary(serverSummary{
		APIHost:      "api.nimbu.io",
		ChildCommand: "yarn dev:server",
		ProxyURL:     "http://127.0.0.1:4568",
		ReadyURL:     "http://127.0.0.1:3456",
		SiteHost:     "demo.nimbu.io",
	})

	got := buf.String()
	if !strings.HasPrefix(got, "\n") {
		t.Fatalf("expected banner spacer line, got: %q", got)
	}
	for _, want := range []string{
		"nimbu-cli",
		"+",
		"_    _",
		"dev server: http://localhost:3456 (proxy -> :4568)",
		"live site: https://demo.nimbu.io",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got: %s", want, got)
		}
	}
	if !strings.Contains(got, "\ndev server: http://localhost:3456 (proxy -> :4568)\n live site: https://demo.nimbu.io\n") {
		t.Fatalf("expected outlined two-line summary, got: %s", got)
	}
	for _, unwanted := range []string{"Ready", "api.nimbu.io", "yarn dev:server", "SiteDisplay"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("did not expect %q in compact output, got: %s", unwanted, got)
		}
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected plain output, got ANSI: %q", got)
	}
}

func TestServerPresenterPrintSummaryShowsNonDefaultAPI(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	presenter := newServerPresenter(ctx, false)
	presenter.PrintSummary(serverSummary{
		APIHost:  "staging-api.nimbu.io",
		ProxyURL: "http://127.0.0.1:4568",
		ReadyURL: "http://127.0.0.1:3456",
		SiteHost: "demo.nimbu.be",
	})

	got := buf.String()
	for _, want := range []string{"dev server: http://localhost:3456 (proxy -> :4568)", "live site: https://demo.nimbu.be", "API", "staging-api.nimbu.io"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got: %s", want, got)
		}
	}
}

func TestServerPresenterPrintSummaryShowsHostForNonLocalPorts(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	presenter := newServerPresenter(ctx, false)
	presenter.PrintSummary(serverSummary{
		ProxyURL: "http://10.0.0.12:4568",
		ReadyURL: "http://10.0.0.12:3456",
		SiteHost: "demo.nimbu.io",
	})

	got := buf.String()
	for _, want := range []string{"dev server: http://10.0.0.12:3456 (proxy -> :4568)", "live site: https://demo.nimbu.io"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got: %s", want, got)
		}
	}
}

func TestServerPresenterPrintSummaryShowsFullProxyHintWhenHostsDiffer(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	presenter := newServerPresenter(ctx, false)
	presenter.PrintSummary(serverSummary{
		ProxyURL: "http://127.0.0.1:4568",
		ReadyURL: "http://10.0.0.12:3456",
		SiteHost: "demo.nimbu.io",
	})

	got := buf.String()
	for _, want := range []string{"dev server: http://10.0.0.12:3456 (proxy -> http://localhost:4568)", "live site: https://demo.nimbu.io"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in mixed-host output, got: %s", want, got)
		}
	}
}

func TestServerPresenterPrintSummaryShowsChildForExceptionalSetup(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	presenter := newServerPresenter(ctx, false)
	presenter.PrintSummary(serverSummary{
		ChildCommand: "yarn dev:server",
		ChildCWD:     "apps/web",
		ProxyURL:     "http://127.0.0.1:4568",
		ReadyURL:     "http://127.0.0.1:3456",
		SiteHost:     "demo.nimbu.io",
	})

	got := buf.String()
	if !strings.Contains(got, "Child") || !strings.Contains(got, "cwd apps/web") {
		t.Fatalf("expected child diagnostic for exceptional setup, got: %s", got)
	}
}

func TestServerPresenterDisabledForMachineModes(t *testing.T) {
	tests := []struct {
		name   string
		mode   output.Mode
		events bool
	}{
		{name: "json", mode: output.Mode{JSON: true}},
		{name: "plain", mode: output.Mode{Plain: true}},
		{name: "events json", events: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			ctx := output.WithMode(context.Background(), tc.mode)
			ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "always"})

			presenter := newServerPresenter(ctx, tc.events)
			presenter.PrintBanner()
			presenter.PrintSummary(serverSummary{SiteHost: "acme.nimbu.io", ProxyURL: "http://127.0.0.1:4568"})
			presenter.PrintShutdownNotice()
			presenter.PrintGoodbye()

			if buf.Len() != 0 {
				t.Fatalf("expected no human output, got: %q", buf.String())
			}
		})
	}
}

func TestFormatSiteSubdomain(t *testing.T) {
	if got := formatSiteSubdomain(api.Site{Subdomain: "acme"}, "fallback"); got != "acme" {
		t.Fatalf("unexpected subdomain: %q", got)
	}
	if got := formatSiteSubdomain(api.Site{}, "fallback"); got != "fallback" {
		t.Fatalf("unexpected fallback subdomain: %q", got)
	}
}

func TestSiteHostFromAPI(t *testing.T) {
	if got := siteHostFromAPI("https://api.nimbu.io", "demo"); got != "demo.nimbu.io" {
		t.Fatalf("unexpected nimbu.io site host: %q", got)
	}
	if got := siteHostFromAPI("https://api.nimbu.be", "demo"); got != "demo.nimbu.be" {
		t.Fatalf("unexpected nimbu.be site host: %q", got)
	}
	if got := siteHostFromAPI("https://custom-api.example.com", "demo"); got != "demo" {
		t.Fatalf("unexpected fallback site host: %q", got)
	}
	if got := siteHostFromAPI("https://api.nimbu.io", "demo.nimbu.io"); got != "demo.nimbu.io" {
		t.Fatalf("unexpected passthrough site host: %q", got)
	}
}

func TestDisplaySiteURL(t *testing.T) {
	if got := displaySiteURL("demo.nimbu.io"); got != "https://demo.nimbu.io" {
		t.Fatalf("unexpected site url: %q", got)
	}
	if got := displaySiteURL("https://demo.nimbu.io"); got != "https://demo.nimbu.io" {
		t.Fatalf("unexpected passthrough site url: %q", got)
	}
}

func TestDisplayPathFromRoot(t *testing.T) {
	if got := displayPathFromRoot("/repo/theme", "/repo/theme"); got != "." {
		t.Fatalf("expected dot path, got %q", got)
	}
	if got := displayPathFromRoot("/repo/theme", "/repo/theme/apps/web"); got != "apps/web" {
		t.Fatalf("expected relative path, got %q", got)
	}
	if got := displayPathFromRoot("/repo/theme", "/outside/project"); got != "/outside/project" {
		t.Fatalf("expected absolute fallback, got %q", got)
	}
}
