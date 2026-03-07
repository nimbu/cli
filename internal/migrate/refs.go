package migrate

import (
	"fmt"
	"net/url"
	"strings"
)

// SiteRef points at one site on one API host.
type SiteRef struct {
	BaseURL string `json:"base_url"`
	Site    string `json:"site"`
}

// ChannelRef points at one channel on one site.
type ChannelRef struct {
	SiteRef
	Channel string `json:"channel"`
}

// ThemeRef points at one theme on one site.
type ThemeRef struct {
	SiteRef
	Theme string `json:"theme"`
}

// ParseSiteRef parses `[site]` plus optional host override.
func ParseSiteRef(raw, hostOverride, defaultSite, defaultBaseURL string) (SiteRef, error) {
	site := strings.TrimSpace(raw)
	if site == "" {
		site = strings.TrimSpace(defaultSite)
	}
	if site == "" {
		return SiteRef{}, fmt.Errorf("site required")
	}
	baseURL, err := normalizeBaseURL(hostOverride, defaultBaseURL)
	if err != nil {
		return SiteRef{}, err
	}
	return SiteRef{BaseURL: baseURL, Site: site}, nil
}

// ParseChannelRef parses `site/channel` or `channel`.
func ParseChannelRef(raw, hostOverride, defaultSite, defaultBaseURL string) (ChannelRef, error) {
	value := strings.Trim(strings.TrimSpace(raw), "/")
	if value == "" {
		return ChannelRef{}, fmt.Errorf("channel reference required")
	}
	parts := strings.SplitN(value, "/", 2)
	site := defaultSite
	channel := value
	if len(parts) == 2 {
		site = parts[0]
		channel = parts[1]
	}
	if strings.TrimSpace(site) == "" {
		return ChannelRef{}, fmt.Errorf("site required in %q", raw)
	}
	if strings.TrimSpace(channel) == "" {
		return ChannelRef{}, fmt.Errorf("channel required in %q", raw)
	}
	ref, err := ParseSiteRef(site, hostOverride, defaultSite, defaultBaseURL)
	if err != nil {
		return ChannelRef{}, err
	}
	ref.Site = site
	return ChannelRef{SiteRef: ref, Channel: channel}, nil
}

// ParseThemeRef parses `site[/theme]`.
func ParseThemeRef(raw, hostOverride, defaultSite, defaultBaseURL string) (ThemeRef, error) {
	value := strings.Trim(strings.TrimSpace(raw), "/")
	if value == "" {
		return ThemeRef{}, fmt.Errorf("theme reference required")
	}
	parts := strings.SplitN(value, "/", 2)
	site := ""
	theme := "default-theme"
	if len(parts) == 1 {
		if defaultSite != "" {
			site = defaultSite
			theme = parts[0]
		} else {
			site = parts[0]
		}
	} else {
		site = parts[0]
		if strings.TrimSpace(parts[1]) != "" {
			theme = parts[1]
		}
	}
	ref, err := ParseSiteRef(site, hostOverride, defaultSite, defaultBaseURL)
	if err != nil {
		return ThemeRef{}, err
	}
	return ThemeRef{SiteRef: ref, Theme: theme}, nil
}

func normalizeBaseURL(override, fallback string) (string, error) {
	value := strings.TrimSpace(override)
	if value == "" {
		value = strings.TrimSpace(fallback)
	}
	if value == "" {
		return "", fmt.Errorf("api url required")
	}
	if !strings.Contains(value, "://") {
		host := strings.TrimPrefix(value, "api.")
		if !strings.HasPrefix(host, "api.") {
			host = "api." + host
		}
		value = "https://" + host
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse api url: %w", err)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid api url: %q", value)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
