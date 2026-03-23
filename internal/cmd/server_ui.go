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
	enabled  bool
	useColor bool
	out      io.Writer
	termOut  *termenv.Output
	palette  []string
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
	useColor := enabled && writer.UseColor()

	profile := termenv.Ascii
	if useColor {
		switch writer.Color {
		case "always":
			profile = termenv.TrueColor
		default:
			profile = termenv.EnvColorProfile()
		}
	}

	palette := DefaultBannerPalette()
	if cfg, ok := ctx.Value(configKey{}).(*config.Config); ok && cfg != nil && cfg.BannerTheme != "" {
		if theme, found := BannerThemeByName(cfg.BannerTheme); found {
			palette = theme.Palette
		}
	}

	return &serverPresenter{
		enabled:  enabled,
		useColor: useColor,
		out:      out,
		termOut:  termenv.NewOutput(out, termenv.WithProfile(profile)),
		palette:  palette,
	}
}

func (p *serverPresenter) Enabled() bool {
	return p != nil && p.enabled
}

func (p *serverPresenter) UseColor() bool {
	return p != nil && p.useColor
}

func (p *serverPresenter) PrintBanner() {
	if !p.Enabled() {
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

func (p *serverPresenter) PrintShutdownNotice() {
	if !p.Enabled() {
		return
	}
	_, _ = fmt.Fprintln(p.out, p.dim("Shutting down..."))
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
