package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// SitesSettingsCmd fetches site settings.
type SitesSettingsCmd struct{}

// Run executes the settings command.
func (c *SitesSettingsCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	var settings any
	path := "/sites/" + url.PathEscape(site) + "/settings"
	if err := client.Get(ctx, path, &settings); err != nil {
		return fmt.Errorf("get site settings: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, settings)
	}

	if mode.Plain {
		// best-effort single-field representation for scripts
		return output.Plain(ctx, site, fmt.Sprintf("%T", settings))
	}

	if _, err := output.Fprintf(ctx, "Site settings for: %s\n", site); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Type:         %T\n", settings); err != nil {
		return err
	}
	return nil
}
