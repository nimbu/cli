package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// siteDetail enriches api.Site with computed URLs for JSON output.
type siteDetail struct {
	api.Site
	AdminURL string `json:"admin_url,omitempty"`
	LiveURL  string `json:"live_url,omitempty"`
}

// SitesGetCmd gets site details.
type SitesGetCmd struct {
	Site string `arg:"" optional:"" help:"Site ID or subdomain (defaults to current site)"`
}

// Run executes the get command.
func (c *SitesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, c.Site)
	if err != nil {
		return err
	}

	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	var s api.Site
	path := "/sites/" + url.PathEscape(site)
	if err := client.Get(ctx, path, &s); err != nil {
		return fmt.Errorf("get site: %w", err)
	}

	var locales string
	if len(s.Locales) > 0 {
		locales = fmt.Sprintf("%v", s.Locales)
	}

	siteHost := siteHostFromAPI(flags.APIURL, s.Subdomain)
	liveURL := displaySiteURL(s.Domain)
	if liveURL == "" {
		liveURL = displaySiteURL(siteHost)
	}
	var adminURL string
	if siteHost != "" {
		adminURL = strings.TrimRight(displaySiteURL(siteHost), "/") + "/admin"
	}

	detail := siteDetail{Site: s, AdminURL: adminURL, LiveURL: liveURL}

	return output.Detail(ctx, detail, []any{s.ID, s.Subdomain, s.Name, s.Domain, liveURL, adminURL}, []output.Field{
		output.FAlways("ID", s.ID),
		output.FAlways("Subdomain", s.Subdomain),
		output.FAlways("Name", s.Name),
		output.F("Domain", s.Domain),
		output.F("Live URL", liveURL),
		output.F("Admin URL", adminURL),
		output.F("Timezone", s.Timezone),
		output.F("Locales", locales),
	})
}
