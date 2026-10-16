package cmd

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/muesli/termenv"

	"github.com/nimbu/cli/internal/output"
)

const serverBanner = "" +
	"███╗   ██╗██╗███╗   ███╗██████╗ ██╗   ██╗\n" +
	"████╗  ██║██║████╗ ████║██╔══██╗██║   ██║\n" +
	"██╔██╗ ██║██║██╔████╔██║██████╔╝██║   ██║\n" +
	"██║╚██╗██║██║██║╚██╔╝██║██╔══██╗██║   ██║\n" +
	"██║ ╚████║██║██║ ╚═╝ ██║██████╔╝╚██████╔╝\n" +
	"╚═╝  ╚═══╝╚═╝╚═╝     ╚═╝╚═════╝  ╚═════╝"

var serverGoodbyeMessages = []string{
	"All quiet. Back to building.",
	"Server down. Keep shipping.",
	"Proxy parked. Templates safe.",
	"Done here. Good luck with the next bug.",
	"Shutting down clean. See you on the next request.",
	"That run is wrapped. Keep coding.",
	"Local stack folded. Back to work.",
}

type serverPresenter struct {
	enabled    bool
	showBanner bool
	out        io.Writer
	// theme styles every presenter line for stderr's color profile, with the
	// palette variant for stderr's background when it is known.
	theme   output.Theme
	palette []string
}

type serverSummary struct {
	APIHost      string
	ChildCommand string
	ChildCWD     string
	ProxyURL     string
	ReadyURL     string
	SiteHost     string
	SiteLabel    string
	Shortcuts    serverShortcutLinks
}

type serverSummarySegment struct {
	dim  bool
	text string
}

// newServerPresenter builds the human chrome for nimbu server and the line
// path of nimbu init. All of it goes to stderr, so stderr decides color and
// ShouldShowBanner decides the banner. When the banner shows on a color
// terminal, the background is queried once (before any program reads stdin)
// and both the banner and the chrome use the matching light or dark palette.
func newServerPresenter(ctx context.Context, eventsJSON bool) *serverPresenter {
	writer := output.WriterFromContext(ctx)
	out := writer.Err
	if out == nil {
		out = os.Stderr
	}

	enabled := output.IsHuman(ctx) && !eventsJSON
	showBanner := enabled && ShouldShowBanner(writer)
	profile := termenv.Ascii
	if enabled {
		profile = writer.ErrColorProfile()
	}

	// Without a banner nothing queries the terminal; the dark palette is the
	// default for an unknown background.
	dark := true
	var palette []string
	if showBanner {
		palette = ResolveBannerPalette(writer, configuredBannerTheme(ctx))
		dark = lookupBannerBackground(writer)
	}

	return &serverPresenter{
		enabled:    enabled,
		showBanner: showBanner,
		out:        out,
		theme:      output.NewThemeFor(profile, dark),
		palette:    palette,
	}
}

// SetOutput redirects presenter lines, e.g. through the server console so
// they never land in the middle of a child output line. Colors stay as decided
// for stderr, so out must end up on stderr.
func (p *serverPresenter) SetOutput(out io.Writer) {
	if p == nil || out == nil {
		return
	}
	p.out = out
}

func (p *serverPresenter) Enabled() bool {
	return p != nil && p.enabled
}

func (p *serverPresenter) PrintBanner() {
	if !p.Enabled() || !p.showBanner {
		return
	}

	lines := strings.Split(strings.Trim(serverBanner, "\n"), "\n")
	label := fmt.Sprintf(" nimbu %s ", serverCLIVersion())
	contentWidth := 0
	for _, line := range lines {
		if w := utf8.RuneCountInString(line); w > contentWidth {
			contentWidth = w
		}
	}
	if minWidth := len(label) - 2; minWidth > contentWidth {
		contentWidth = minWidth
	}

	const pad = 2
	padStr := strings.Repeat(" ", pad)

	innerWidth := contentWidth + 2*pad

	emptyRow := "|" + strings.Repeat(" ", innerWidth) + "|"

	_, _ = fmt.Fprintln(p.out)
	_, _ = fmt.Fprintln(p.out, p.border(framedBannerEdge(innerWidth-2, label)))
	_, _ = fmt.Fprintln(p.out, p.border(emptyRow))
	for i, line := range lines {
		padded := line + strings.Repeat(" ", contentWidth-utf8.RuneCountInString(line))
		_, _ = fmt.Fprintf(p.out, "%s%s%s\n", p.border("|"+padStr), p.bannerLine(padded, i), p.border(padStr+"|"))
	}
	_, _ = fmt.Fprintln(p.out, p.border("+"+strings.Repeat("-", innerWidth)+"+"))
}

func (p *serverPresenter) PrintSummary(summary serverSummary) {
	if !p.Enabled() {
		return
	}

	lines, rows, unusual := compactServerSummaryRows(summary)
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}
		_, _ = fmt.Fprintln(p.out, p.summaryInline(line, i))
	}
	if len(rows) == 0 {
		if child := compactChildInfo(summary, unusual); child != "" {
			_, _ = fmt.Fprintln(p.out, p.summaryRow("Child", child))
		}
		if hint := summary.Shortcuts.Hint(); hint != "" {
			_, _ = fmt.Fprintln(p.out, p.shortcutHint(hint))
		}
		_, _ = fmt.Fprintln(p.out)
		return
	}

	for _, row := range rows {
		if row[1] == "" {
			continue
		}
		_, _ = fmt.Fprintln(p.out, p.summaryRow(row[0], row[1]))
	}
	if child := compactChildInfo(summary, unusual); child != "" {
		_, _ = fmt.Fprintln(p.out, p.summaryRow("Child", child))
	}
	if hint := summary.Shortcuts.Hint(); hint != "" {
		_, _ = fmt.Fprintln(p.out, p.shortcutHint(hint))
	}
	_, _ = fmt.Fprintln(p.out)
}

// PrintStopping acknowledges the first stop request. In machine modes it
// keeps the plain "received signal" line for signal-driven stops.
func (p *serverPresenter) PrintStopping(signalName string) {
	if p == nil {
		return
	}
	if !p.Enabled() {
		if signalName != "" {
			_, _ = fmt.Fprintf(p.out, "received signal: %s; stopping (Ctrl-C again to force)\n", signalName)
		}
		return
	}
	_, _ = fmt.Fprintln(p.out, p.dim("Stopping… (Ctrl-C again to force)"))
}

// PrintForceStop acknowledges the second stop request.
func (p *serverPresenter) PrintForceStop() {
	if p == nil {
		return
	}
	if !p.Enabled() {
		_, _ = fmt.Fprintln(p.out, "forcing stop")
		return
	}
	_, _ = fmt.Fprintln(p.out, p.dim("Forcing stop."))
}

// PrintReady marks the moment both the proxy and the child answer.
func (p *serverPresenter) PrintReady(target string) {
	if !p.Enabled() || strings.TrimSpace(target) == "" {
		return
	}
	_, _ = fmt.Fprintf(p.out, "  %s %s\n", p.accent(output.GlyphSet(output.ASCIIOnly()).OK+" ready"), p.value(target))
}

// PrintShortcutHelp prints the full key list ('?' / 'h').
func (p *serverPresenter) PrintShortcutHelp(links serverShortcutLinks) {
	if !p.Enabled() {
		return
	}
	_, _ = fmt.Fprintln(p.out, p.shortcutHint("Shortcuts"))
	for _, row := range links.HelpRows() {
		key := p.theme.Strong(fmt.Sprintf("%-6s", row[0]))
		_, _ = fmt.Fprintf(p.out, "    %s %s\n", key, p.dim(row[1]))
	}
}

func (p *serverPresenter) PrintGoodbye() {
	if !p.Enabled() {
		return
	}
	_, _ = fmt.Fprintln(p.out, p.accent(randomServerGoodbye()))
}

func (p *serverPresenter) PrintShortcutPending() {
	if !p.Enabled() {
		return
	}
	_, _ = fmt.Fprintln(p.out, p.shortcutHint("shortcuts available after ready"))
}

func (p *serverPresenter) PrintShortcutError(message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	if p.Enabled() {
		_, _ = fmt.Fprintln(p.out, p.shortcutHint(message))
		return
	}
	_, _ = fmt.Fprintln(p.out, message)
}

// bannerLine colors banner row idx; an empty palette entry keeps the
// terminal's own text color.
func (p *serverPresenter) bannerLine(line string, idx int) string {
	if line == "" || len(p.palette) == 0 {
		return line
	}
	return p.theme.Paint(line, p.palette[idx%len(p.palette)], true)
}

func (p *serverPresenter) accent(value string) string { return p.theme.Success(value) }

func (p *serverPresenter) dim(value string) string { return p.theme.Muted(value) }

// value keeps the terminal's own foreground: a fixed near-white disappears on
// light backgrounds.
func (p *serverPresenter) value(value string) string { return value }

func (p *serverPresenter) summaryRow(label string, value string) string {
	padded := p.theme.Paint(fmt.Sprintf("%-9s", label), p.theme.Palette().Muted, true)
	return fmt.Sprintf("  %s %s", padded, p.value(value))
}

func (p *serverPresenter) summaryInline(segments []serverSummarySegment, index int) string {
	var b strings.Builder
	if index > 0 {
		b.WriteString(" ")
	}
	for _, segment := range segments {
		if segment.dim {
			b.WriteString(p.dim(segment.text))
			continue
		}
		b.WriteString(p.value(segment.text))
	}
	return b.String()
}

func (p *serverPresenter) shortcutHint(value string) string {
	return "  " + p.dim(value)
}

func (p *serverPresenter) border(value string) string { return p.theme.Border(value) }

func randomServerGoodbye() string {
	return serverGoodbyeMessages[rand.IntN(len(serverGoodbyeMessages))]
}

func serverCLIVersion() string {
	v := strings.TrimSpace(version)
	if v == "" {
		v = "dev"
	}
	shortCommit := shortCommitHash(commit)
	if shortCommit == "" || shortCommit == "none" {
		return v
	}
	if v == "dev" || strings.Contains(v, "dev") {
		return fmt.Sprintf("%s (%s)", v, shortCommit)
	}
	return v
}

func shortCommitHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if hash == "" || hash == "none" {
		return ""
	}
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func framedBannerEdge(contentWidth int, label string) string {
	innerWidth := contentWidth + 2
	if len(label) > innerWidth {
		innerWidth = len(label)
	}
	left := (innerWidth - len(label)) / 2
	right := innerWidth - len(label) - left
	return "+" + strings.Repeat("-", left) + label + strings.Repeat("-", right) + "+"
}
