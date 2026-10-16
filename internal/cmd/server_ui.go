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

	"github.com/nimbu/cli/internal/config"
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
	useColor   bool
	out        io.Writer
	termOut    *termenv.Output
	palette    []string
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

func newServerPresenter(ctx context.Context, eventsJSON bool) *serverPresenter {
	writer := output.WriterFromContext(ctx)
	out := writer.Err
	if out == nil {
		out = os.Stderr
	}

	enabled := output.IsHuman(ctx) && !eventsJSON
	// All presenter chrome goes to stderr, so the stderr decision applies.
	useColor := enabled && writer.ErrUseColor()
	profile := termenv.Ascii
	if useColor {
		profile = writer.ErrColorProfile()
	}

	palette := DefaultBannerPalette()
	if cfg, ok := ctx.Value(configKey{}).(*config.Config); ok && cfg != nil && cfg.BannerTheme != "" {
		if theme, found := BannerThemeByName(cfg.BannerTheme); found {
			palette = theme.Palette
		}
	}

	return &serverPresenter{
		enabled:    enabled,
		showBanner: enabled,
		useColor:   useColor,
		out:        out,
		termOut:    termenv.NewOutput(out, termenv.WithProfile(profile)),
		palette:    palette,
	}
}

// serverBannerAllowed reports whether the block-letter banner fits: it is
// decoration for an interactive terminal, so it is skipped when stderr is
// piped or captured, and in CI logs.
func serverBannerAllowed(stderrTTY bool, getenv func(string) string) bool {
	if !stderrTTY {
		return false
	}
	ci := strings.ToLower(strings.TrimSpace(getenv("CI")))
	return ci == "" || ci == "0" || ci == "false"
}

// LimitBannerToTerminal hides the banner when stderr is not a terminal or CI
// is set. nimbu server opts in; nimbu init shares the presenter and keeps its
// own behavior.
func (p *serverPresenter) LimitBannerToTerminal(stderrTTY bool) {
	if p == nil {
		return
	}
	p.showBanner = p.showBanner && serverBannerAllowed(stderrTTY, os.Getenv)
}

// SetOutput redirects presenter lines, e.g. through the server console so
// they never land in the middle of a child output line. Colors stay as decided
// for stderr, so out must end up on stderr.
func (p *serverPresenter) SetOutput(out io.Writer) {
	if p == nil || out == nil {
		return
	}
	p.out = out
	p.termOut = termenv.NewOutput(out, termenv.WithProfile(p.termOut.Profile))
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
	_, _ = fmt.Fprintln(p.out, p.borderLine(framedBannerEdge(innerWidth-2, label)))
	_, _ = fmt.Fprintln(p.out, p.borderLine(emptyRow))
	for i, line := range lines {
		padded := line + strings.Repeat(" ", contentWidth-utf8.RuneCountInString(line))
		if p.useColor {
			_, _ = fmt.Fprintf(p.out, "%s%s%s\n", p.border("|"+padStr), p.bannerLine(padded, i), p.border(padStr+"|"))
			continue
		}
		_, _ = fmt.Fprintf(p.out, "|%s%s%s|\n", padStr, padded, padStr)
	}
	_, _ = fmt.Fprintln(p.out, p.borderLine("+"+strings.Repeat("-", innerWidth)+"+"))
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

// PrintStopping acknowledges the first stop request; forceHint says how to
// force the stop (see serverForceHint). In machine modes it keeps the plain
// "received signal" line for signal-driven stops.
func (p *serverPresenter) PrintStopping(signalName, forceHint string) {
	if p == nil {
		return
	}
	suffix := ""
	if forceHint != "" {
		suffix = " (" + forceHint + ")"
	}
	if !p.Enabled() {
		if signalName != "" {
			_, _ = fmt.Fprintf(p.out, "received signal: %s; stopping%s\n", signalName, suffix)
		}
		return
	}
	_, _ = fmt.Fprintln(p.out, p.dim("Stopping…"+suffix))
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
	_, _ = fmt.Fprintf(p.out, "  %s %s\n", p.accent("✓ ready"), p.value(target))
}

// PrintShortcutHelp prints the full key list ('?' / 'h').
func (p *serverPresenter) PrintShortcutHelp(links serverShortcutLinks) {
	if !p.Enabled() {
		return
	}
	_, _ = fmt.Fprintln(p.out, p.shortcutHint("Shortcuts"))
	for _, row := range links.HelpRows() {
		key := fmt.Sprintf("%-6s", row[0])
		if p.useColor {
			key = p.termOut.String(key).Bold().String()
		}
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

func (p *serverPresenter) bannerLine(line string, idx int) string {
	if !p.useColor || line == "" || len(p.palette) == 0 {
		return line
	}
	color := p.palette[idx%len(p.palette)]
	return p.termOut.String(line).Foreground(p.termOut.Color(color)).Bold().String()
}

func (p *serverPresenter) accent(value string) string {
	if !p.useColor {
		return value
	}
	return p.termOut.String(value).Foreground(p.termOut.Color("#22c55e")).Bold().String()
}

func (p *serverPresenter) dim(value string) string {
	if !p.useColor {
		return value
	}
	return p.termOut.String(value).Foreground(p.termOut.Color("#94a3b8")).String()
}

func (p *serverPresenter) value(value string) string {
	if !p.useColor {
		return value
	}
	return p.termOut.String(value).Foreground(p.termOut.Color("#e2e8f0")).String()
}

func (p *serverPresenter) summaryRow(label string, value string) string {
	padded := fmt.Sprintf("%-9s", label)
	if p.useColor {
		padded = p.termOut.String(padded).Foreground(p.termOut.Color("#94a3b8")).Bold().String()
	}
	return fmt.Sprintf("  %s %s", padded, p.value(value))
}

func (p *serverPresenter) summaryInline(segments []serverSummarySegment, index int) string {
	var b strings.Builder
	if index > 0 {
		b.WriteString(" ")
	}
	for _, segment := range segments {
		if p.useColor {
			if segment.dim {
				b.WriteString(p.dim(segment.text))
				continue
			}
			b.WriteString(p.value(segment.text))
			continue
		}
		b.WriteString(segment.text)
	}
	return b.String()
}

func (p *serverPresenter) shortcutHint(value string) string {
	if p.useColor {
		value = p.termOut.String(value).Foreground(p.termOut.Color("#94a3b8")).String()
	}
	return "  " + value
}

func (p *serverPresenter) border(value string) string {
	if !p.useColor {
		return value
	}
	return p.termOut.String(value).Foreground(p.termOut.Color("#64748b")).String()
}

func (p *serverPresenter) borderLine(value string) string {
	if !p.useColor {
		return value
	}
	return p.termOut.String(value).Foreground(p.termOut.Color("#64748b")).String()
}

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
