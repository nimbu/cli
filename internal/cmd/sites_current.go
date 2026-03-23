package cmd

import (
	"context"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// SitesCurrentCmd shows the current site context.
type SitesCurrentCmd struct{}

// Run executes the current command.
func (c *SitesCurrentCmd) Run(ctx context.Context, flags *RootFlags) error {
	var site, source string

	// Check flag first
	if flags.Site != "" {
		site = flags.Site
		source = "flag"
	}

	// Check config
	if site == "" {
		cfg, err := config.Read()
		if err == nil && cfg.DefaultSite != "" {
			site = cfg.DefaultSite
			source = "config"
		}
	}

	// Check project file
	if site == "" {
		if proj, err := config.ReadProjectConfig(); err == nil && proj.Site != "" {
			site = proj.Site
			source = "project"
		}
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]string{
			"site":   site,
			"source": source,
		})
	}

	if mode.Plain {
		return output.Plain(ctx, site, source)
	}

	if site == "" {
		if _, err := output.Fprintln(ctx, "No site configured"); err != nil {
			return err
		}
		if _, err := output.Fprintln(ctx); err != nil {
			return err
		}
		if _, err := output.Fprintln(ctx, "Set a site using:"); err != nil {
			return err
		}
		if _, err := output.Fprintln(ctx, "  --site flag"); err != nil {
			return err
		}
		if _, err := output.Fprintln(ctx, "  NIMBU_SITE environment variable"); err != nil {
			return err
		}
		if _, err := output.Fprintln(ctx, "  default_site in config file"); err != nil {
			return err
		}
		if _, err := output.Fprintln(ctx, "  site in nimbu.yml project file"); err != nil {
			return err
		}
		return nil
	}

	return output.Detail(ctx, map[string]string{"site": site, "source": source},
		[]any{site, source},
		[]output.Field{
			output.FAlways("Site", site),
			output.FAlways("Source", source),
		},
	)
}
