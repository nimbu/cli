package cmd

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/config"
)

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
		segments := make([]serverSummarySegment, 0, 6)
		segments = append(segments,
			serverSummarySegment{dim: true, text: "dev server: "},
			serverSummarySegment{text: primaryURL},
		)
		if hint := proxyHint(summary.ProxyURL, summary.ReadyURL); hint != "" {
			if summary.SiteLabel != "" {
				segments = append(segments,
					serverSummarySegment{dim: true, text: " (" + hint + " -> "},
					serverSummarySegment{text: summary.SiteLabel},
					serverSummarySegment{dim: true, text: ")"},
				)
			} else {
				segments = append(segments, serverSummarySegment{dim: true, text: " (" + hint + ")"})
			}
		}
		lines = append(lines, segments)
		if summary.SiteHost != "" && summary.SiteLabel == "" {
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
	return host != "" && host != config.DefaultAPIHost
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
			return "-> " + display
		}
		return ""
	}
	primary := parseServerEndpoint(primaryURL)
	if !primary.ok || sameDisplayHost(proxy, primary) {
		return "-> :" + proxy.port
	}
	return "-> " + displayServerURL(proxyURL)
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

func compactSiteLabel(site string) string {
	site = strings.TrimSpace(site)
	if site == "" || strings.Contains(site, ".") {
		return ""
	}
	return site
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
