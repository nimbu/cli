package cmd

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muesli/termenv"

	"github.com/nimbu/cli/internal/api"
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

const defaultServerAPIHost = "api.nimbu.io"

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

func serverAPIHost(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return strings.TrimSpace(baseURL)
	}
	return parsed.Host
}

func formatServerChildCommand(command string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	if command != "" {
		parts = append(parts, quoteCommandPart(command))
	}
	for _, arg := range args {
		parts = append(parts, quoteCommandPart(arg))
	}
	line := strings.Join(parts, " ")
	const maxLen = 88
	if len(line) > maxLen {
		return line[:maxLen-3] + "..."
	}
	return line
}

func quoteCommandPart(part string) string {
	if part == "" {
		return `""`
	}
	if strings.ContainsAny(part, " \t\n\"'") {
		return strconv.Quote(part)
	}
	return part
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

func displayPathFromRoot(projectRoot string, target string) string {
	if target == "" {
		return ""
	}
	if projectRoot == "" {
		return target
	}
	rel, err := filepath.Rel(projectRoot, target)
	if err != nil {
		return target
	}
	if rel == "." {
		return "."
	}
	if strings.HasPrefix(rel, "..") {
		return target
	}
	return rel
}

func lookupSiteSubdomain(ctx context.Context, client *api.Client, fallback string) string {
	if client == nil || fallback == "" {
		return fallback
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	var site api.Site
	path := "/sites/" + url.PathEscape(fallback)
	if err := client.Get(lookupCtx, path, &site); err != nil {
		return fallback
	}
	return formatSiteSubdomain(site, fallback)
}

func formatSiteSubdomain(site api.Site, fallback string) string {
	subdomain := strings.TrimSpace(site.Subdomain)
	if subdomain != "" {
		return subdomain
	}
	return fallback
}

type serverEndpoint struct {
	host  string
	local bool
	ok    bool
	port  string
	raw   string
}

func compactServerSummaryRows(summary serverSummary) ([][]serverSummarySegment, [][2]string, bool) {
	lines, unusual := compactPrimarySummary(summary)
	rows := make([][2]string, 0, 1)

	if shouldShowServerAPI(summary.APIHost) {
		rows = append(rows, [2]string{"API", summary.APIHost})
	}

	return lines, rows, unusual
}

func compactPrimarySummary(summary serverSummary) ([][]serverSummarySegment, bool) {
	lines := make([][]serverSummarySegment, 0, 2)
	primaryURL := displayServerURL(summary.ReadyURL)
	if primaryURL != "" {
		segments := make([]serverSummarySegment, 0, 4)
		segments = append(segments,
			serverSummarySegment{dim: true, text: "dev server: "},
			serverSummarySegment{text: primaryURL},
		)
		if hint := proxyHint(summary.ProxyURL, summary.ReadyURL); hint != "" {
			segments = append(segments, serverSummarySegment{dim: true, text: " (" + hint + ")"})
		}
		lines = append(lines, segments)
		if summary.SiteHost != "" {
			lines = append(lines, []serverSummarySegment{
				{dim: true, text: "live site: "},
				{text: displaySiteURL(summary.SiteHost)},
			})
		}
		return lines, false
	}

	proxyURL := displayServerURL(summary.ProxyURL)
	if proxyURL == "" {
		return lines, true
	}

	segments := make([]serverSummarySegment, 0, 5)
	segments = append(segments,
		serverSummarySegment{dim: true, text: "proxy: "},
		serverSummarySegment{text: proxyURL},
	)
	lines = append(lines, segments)
	if summary.SiteHost != "" {
		lines = append(lines, []serverSummarySegment{
			{dim: true, text: "live site: "},
			{text: displaySiteURL(summary.SiteHost)},
		})
	}
	return lines, true
}

func parseServerEndpoint(raw string) serverEndpoint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return serverEndpoint{}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return serverEndpoint{raw: raw}
	}
	port := parsed.Port()
	if port == "" {
		switch strings.ToLower(parsed.Scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" || port == "" {
		return serverEndpoint{raw: raw}
	}
	return serverEndpoint{
		host:  host,
		local: isLocalServerHost(host),
		ok:    true,
		port:  port,
		raw:   raw,
	}
}

func sameDisplayHost(a serverEndpoint, b serverEndpoint) bool {
	if !a.ok || !b.ok {
		return false
	}
	if a.local && b.local {
		return true
	}
	return a.host == b.host
}

func isLocalServerHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func shouldShowServerAPI(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host != "" && host != defaultServerAPIHost
}

func displayServerURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	host := parsed.Hostname()
	if isLocalServerHost(host) {
		if port := parsed.Port(); port != "" {
			parsed.Host = "localhost:" + port
		} else {
			parsed.Host = "localhost"
		}
	}
	return parsed.String()
}

func proxyHint(proxyURL string, primaryURL string) string {
	proxy := parseServerEndpoint(proxyURL)
	if !proxy.ok {
		if display := displayServerURL(proxyURL); display != "" {
			return "proxy -> " + display
		}
		return ""
	}
	primary := parseServerEndpoint(primaryURL)
	if !primary.ok || sameDisplayHost(proxy, primary) {
		return "proxy -> :" + proxy.port
	}
	return "proxy -> " + displayServerURL(proxyURL)
}

func siteHostFromAPI(baseURL string, subdomain string) string {
	subdomain = strings.TrimSpace(subdomain)
	if subdomain == "" {
		return ""
	}
	if strings.Contains(subdomain, ".") {
		return subdomain
	}

	host := serverAPIHost(baseURL)
	host = strings.ToLower(strings.TrimSpace(host))
	if strings.HasPrefix(host, "api.") && len(host) > len("api.") {
		return subdomain + "." + strings.TrimPrefix(host, "api.")
	}
	return subdomain
}

func displaySiteURL(siteHost string) string {
	siteHost = strings.TrimSpace(siteHost)
	if siteHost == "" {
		return ""
	}
	if strings.Contains(siteHost, "://") {
		return siteHost
	}
	return "https://" + siteHost
}

func compactChildInfo(summary serverSummary, unusual bool) string {
	if summary.ChildCommand == "" {
		return ""
	}
	if !unusual && (summary.ChildCWD == "" || summary.ChildCWD == ".") {
		return ""
	}
	if summary.ChildCWD != "" && summary.ChildCWD != "." {
		return fmt.Sprintf("%s · cwd %s", summary.ChildCommand, summary.ChildCWD)
	}
	return summary.ChildCommand
}
