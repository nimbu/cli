package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type themeGetBundle struct {
	Theme api.Theme `json:"theme"`
}

// ThemesGetCmd gets theme details.
type ThemesGetCmd struct {
	Theme string `arg:"" help:"Theme ID"`
}

// Run executes the get command.
func (c *ThemesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var bundle themeGetBundle
	path := "/themes/" + url.PathEscape(c.Theme)
	if err := client.Get(ctx, path, &bundle); err != nil {
		return fmt.Errorf("get theme: %w", err)
	}
	theme := bundle.Theme
	info, err := fetchThemeInfo(ctx, client, c.Theme)
	if err == nil {
		applyThemeInfo(&theme, info)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, theme)
	}

	if mode.Plain {
		return output.Plain(ctx, theme.ID, theme.Name, theme.Active)
	}

	if _, err := output.Fprintf(ctx, "ID:     %s\n", theme.ID); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Name:   %s\n", theme.Name); err != nil {
		return err
	}
	if theme.ThemeShortID != "" {
		if _, err := output.Fprintf(ctx, "Short:  %s\n", theme.ThemeShortID); err != nil {
			return err
		}
	}
	if theme.CDNRoot != "" {
		if _, err := output.Fprintf(ctx, "CDN Root: %s\n", theme.CDNRoot); err != nil {
			return err
		}
	}
	if theme.CDNHost != "" {
		if _, err := output.Fprintf(ctx, "CDN Host: %s\n", theme.CDNHost); err != nil {
			return err
		}
	}
	if theme.CDNBasePath != "" {
		if _, err := output.Fprintf(ctx, "CDN Path: %s\n", theme.CDNBasePath); err != nil {
			return err
		}
	}
	if _, err := output.Fprintf(ctx, "Active: %v\n", theme.Active); err != nil {
		return err
	}
	if theme.SiteID != "" {
		if _, err := output.Fprintf(ctx, "Site ID: %s\n", theme.SiteID); err != nil {
			return err
		}
	}
	if theme.SiteShortID != "" {
		if _, err := output.Fprintf(ctx, "Site Short: %s\n", theme.SiteShortID); err != nil {
			return err
		}
	}
	if !theme.CreatedAt.IsZero() {
		if _, err := output.Fprintf(ctx, "Created: %s\n", theme.CreatedAt.Format("2006-01-02 15:04:05")); err != nil {
			return err
		}
	}
	if !theme.UpdatedAt.IsZero() {
		if _, err := output.Fprintf(ctx, "Updated: %s\n", theme.UpdatedAt.Format("2006-01-02 15:04:05")); err != nil {
			return err
		}
	}

	return nil
}
