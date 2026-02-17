package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

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

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, s)
	}

	if mode.Plain {
		return output.Plain(ctx, s.ID, s.Subdomain, s.Name, s.Domain)
	}

	fmt.Printf("ID:        %s\n", s.ID)
	fmt.Printf("Subdomain: %s\n", s.Subdomain)
	fmt.Printf("Name:      %s\n", s.Name)
	if s.Domain != "" {
		fmt.Printf("Domain:    %s\n", s.Domain)
	}
	if s.Timezone != "" {
		fmt.Printf("Timezone:  %s\n", s.Timezone)
	}
	if len(s.Locales) > 0 {
		fmt.Printf("Locales:   %v\n", s.Locales)
	}

	return nil
}
