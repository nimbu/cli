package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestServerPresenterPrintSummary(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	forceBannerTerminal(t, true)
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
		"nimbu",
		"+",
		"██╗",
		"dev server: http://localhost:3456 (-> :4568)",
		"live site: https://demo.nimbu.io",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got: %s", want, got)
		}
	}
	if !strings.Contains(got, "\ndev server: http://localhost:3456 (-> :4568)\n live site: https://demo.nimbu.io\n") {
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
	for _, want := range []string{"dev server: http://localhost:3456 (-> :4568)", "live site: https://demo.nimbu.be", "API", "staging-api.nimbu.io"} {
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
	for _, want := range []string{"dev server: http://10.0.0.12:3456 (-> :4568)", "live site: https://demo.nimbu.io"} {
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
	for _, want := range []string{"dev server: http://10.0.0.12:3456 (-> http://localhost:4568)", "live site: https://demo.nimbu.io"} {
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

func TestServerPresenterPrintSummaryShowsShortcutHint(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	presenter := newServerPresenter(ctx, false)
	summary := serverSummary{
		ProxyURL:  "http://127.0.0.1:4568",
		ReadyURL:  "http://127.0.0.1:3456",
		SiteHost:  "demo.nimbu.io",
		SiteLabel: "demo",
	}
	summary.Shortcuts = serverShortcutLinksFromSummary(summary)
	summary.Shortcuts.Active = true
	presenter.PrintSummary(summary)

	got := buf.String()
	if !strings.Contains(got, "dev server: http://localhost:3456 (-> :4568 -> demo)") {
		t.Fatalf("expected compact dev/proxy/site line, got: %s", got)
	}
	if strings.Contains(got, "live site:") {
		t.Fatalf("did not expect separate live site line, got: %s", got)
	}
	if !strings.Contains(got, "[o] open dev  [l] open live  [b] open admin  [enter] mark  [?] help  [q] quit") {
		t.Fatalf("expected shortcut hint in output, got: %s", got)
	}
}

func TestServerPresenterPrintSummaryKeepsLiveSiteForCustomHost(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	presenter := newServerPresenter(ctx, false)
	summary := serverSummary{
		ProxyURL: "http://127.0.0.1:4568",
		ReadyURL: "http://127.0.0.1:3456",
		SiteHost: "preview.custom.example.com",
	}
	summary.Shortcuts = serverShortcutLinksFromSummary(summary)
	summary.Shortcuts.Active = true
	presenter.PrintSummary(summary)

	got := buf.String()
	if !strings.Contains(got, "dev server: http://localhost:3456 (-> :4568)") {
		t.Fatalf("expected compact dev/proxy line, got: %s", got)
	}
	if !strings.Contains(got, "live site: https://preview.custom.example.com") {
		t.Fatalf("expected separate live site line, got: %s", got)
	}
	if strings.Contains(got, "-> :4568 -> preview") {
		t.Fatalf("did not expect inline custom host label, got: %s", got)
	}
}

func TestServerPresenterPrintSummaryHidesDevShortcutWithoutReadyURL(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &buf, Err: &buf, Color: "never"})

	presenter := newServerPresenter(ctx, false)
	summary := serverSummary{
		ProxyURL: "http://127.0.0.1:4568",
		SiteHost: "demo.nimbu.io",
	}
	summary.Shortcuts = serverShortcutLinksFromSummary(summary)
	summary.Shortcuts.Active = true
	presenter.PrintSummary(summary)

	got := buf.String()
	if strings.Contains(got, "[o] open dev") {
		t.Fatalf("did not expect dev shortcut in output, got: %s", got)
	}
	if !strings.Contains(got, "[l] open live  [b] open admin  [enter] mark") {
		t.Fatalf("expected live/admin shortcuts in output, got: %s", got)
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
			presenter.PrintStopping("", "")
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

func TestCompactSiteLabel(t *testing.T) {
	if got := compactSiteLabel("demo"); got != "demo" {
		t.Fatalf("unexpected simple site label: %q", got)
	}
	if got := compactSiteLabel("demo.nimbu.io"); got != "" {
		t.Fatalf("expected qualified host to be excluded, got: %q", got)
	}
	if got := compactSiteLabel(""); got != "" {
		t.Fatalf("expected empty site label, got: %q", got)
	}
}

func TestDisplayPathFromRoot(t *testing.T) {
	root := filepath.FromSlash("/repo/theme")
	if got := displayPathFromRoot(root, root); got != "." {
		t.Fatalf("expected dot path, got %q", got)
	}
	if got := displayPathFromRoot(root, filepath.FromSlash("/repo/theme/apps/web")); got != filepath.FromSlash("apps/web") {
		t.Fatalf("expected relative path, got %q", got)
	}
	outside := filepath.FromSlash("/outside/project")
	if got := displayPathFromRoot(root, outside); got != outside {
		t.Fatalf("expected absolute fallback, got %q", got)
	}
}

func TestFormatServerChildCommandTruncatesByCellWidth(t *testing.T) {
	short := formatServerChildCommand("yarn", []string{"dev"})
	if short != "yarn dev" {
		t.Fatalf("short command = %q", short)
	}

	// 'é' is two bytes; a byte cut at 85 would split it into invalid UTF-8.
	args := []string{strings.Repeat("é", 120)}
	got := formatServerChildCommand("node", args)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated command is not valid UTF-8: %q", got)
	}
	if w := ansi.StringWidth(got); w != 88 {
		t.Fatalf("width = %d, want 88: %q", w, got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis tail, got %q", got)
	}

	// Wide runes take two cells each; the result must still fit 88 columns.
	wide := formatServerChildCommand("node", []string{strings.Repeat("界", 80)})
	if w := ansi.StringWidth(wide); w > 88 {
		t.Fatalf("wide width = %d, want <= 88: %q", w, wide)
	}
	if !utf8.ValidString(wide) {
		t.Fatalf("wide command is not valid UTF-8: %q", wide)
	}
}

// fakeBannerBackground answers the background lookup without a terminal.
func fakeBannerBackground(t *testing.T, dark bool) {
	t.Helper()
	orig := lookupBannerBackground
	lookupBannerBackground = func(*output.Writer) bool { return dark }
	t.Cleanup(func() { lookupBannerBackground = orig })
}

func serverColorContext(t *testing.T, buf *bytes.Buffer, theme string) context.Context {
	t.Helper()
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")
	cfg := config.Defaults()
	cfg.BannerTheme = theme
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: buf, Err: buf, Color: "always"})
	return context.WithValue(ctx, configKey{}, &cfg)
}

// The line-path banner follows the background: a light terminal gets the
// theme's light palette and the light chrome palette.
func TestServerPresenterBannerUsesLightPaletteOnLightBackground(t *testing.T) {
	var buf bytes.Buffer
	ctx := serverColorContext(t, &buf, "sakura")
	forceBannerTerminal(t, true)
	fakeBannerBackground(t, false)

	newServerPresenter(ctx, false).PrintBanner()
	theme, _ := BannerThemeByName("sakura")
	got := buf.String()
	if !strings.Contains(got, rgbSGR(theme.Light[0])) {
		t.Fatalf("banner does not use the light palette %v:\n%q", theme.Light, got)
	}
	if strings.Contains(got, rgbSGR(theme.Palette[0])) {
		t.Fatal("banner still uses the dark palette on a light background")
	}

	presenter := newServerPresenter(ctx, false)
	presenter.PrintBanner()
	buf.Reset()
	presenter.PrintStopping("", "")
	if !strings.Contains(buf.String(), rgbSGR(output.PaletteFor(false).Muted)) {
		t.Fatalf("chrome does not use the light muted color: %q", buf.String())
	}
}

// Building the presenter never asks the terminal anything: nimbu server
// builds it before checking the site and login, and a terminal that never
// answers would stall even a command that is about to fail. The query waits
// for PrintBanner.
func TestServerPresenterDefersBackgroundQueryToBanner(t *testing.T) {
	var buf bytes.Buffer
	ctx := serverColorContext(t, &buf, "sakura")
	forceBannerTerminal(t, true)
	queried := 0
	orig := lookupBannerBackground
	lookupBannerBackground = func(*output.Writer) bool {
		queried++
		return false
	}
	t.Cleanup(func() { lookupBannerBackground = orig })

	presenter := newServerPresenter(ctx, false)
	if queried != 0 {
		t.Fatalf("background queried %d times while building the presenter", queried)
	}
	presenter.PrintBanner()
	if queried == 0 {
		t.Fatal("PrintBanner did not resolve the background")
	}
}

// Without the banner (not a terminal, CI) nothing queries the background.
func TestServerPresenterSkipsBackgroundQueryWithoutBanner(t *testing.T) {
	var buf bytes.Buffer
	ctx := serverColorContext(t, &buf, "sakura")
	forceBannerTerminal(t, false)
	orig := lookupBannerBackground
	lookupBannerBackground = func(*output.Writer) bool {
		t.Fatal("background queried without a banner")
		return true
	}
	t.Cleanup(func() { lookupBannerBackground = orig })
	newServerPresenter(ctx, false).PrintBanner()
}

// Values keep the terminal's foreground (a near-white hex vanished on light
// backgrounds), and on a 256-color terminal no 24-bit sequence is emitted.
func TestServerPresenterChromeFollowsProfileAndKeepsTextColor(t *testing.T) {
	var buf bytes.Buffer
	ctx := serverColorContext(t, &buf, "rainbow")
	t.Setenv("COLORTERM", "")
	forceBannerTerminal(t, true)
	fakeBannerBackground(t, true)

	presenter := newServerPresenter(ctx, false)
	presenter.PrintBanner()
	presenter.PrintSummary(serverSummary{
		APIHost:  "staging-api.nimbu.io",
		ProxyURL: "http://127.0.0.1:4568",
		ReadyURL: "http://127.0.0.1:3456",
		SiteHost: "demo.nimbu.io",
	})
	presenter.PrintReady("http://localhost:4568")
	got := buf.String()
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected colored chrome, got %q", got)
	}
	if strings.Contains(got, "38;2;") {
		t.Fatalf("256-color terminal got 24-bit sequences: %q", got)
	}
	if !strings.Contains(got, " staging-api.nimbu.io\n") {
		t.Fatalf("summary value should be uncolored, got %q", got)
	}
}

// rgbSGR is the 24-bit foreground parameter for hex, without the terminator
// (termenv may append bold to the same sequence).
func rgbSGR(hex string) string {
	return strings.TrimSuffix(truecolorFG(hex), "m")
}
