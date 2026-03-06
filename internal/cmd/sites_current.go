package cmd

import (
	"context"
	"fmt"

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
		fmt.Println("No site configured")
		fmt.Println("")
		fmt.Println("Set a site using:")
		fmt.Println("  --site flag")
		fmt.Println("  NIMBU_SITE environment variable")
		fmt.Println("  default_site in config file")
		fmt.Println("  site in nimbu.yml project file")
		return nil
	}

	fmt.Printf("Site:   %s\n", site)
	fmt.Printf("Source: %s\n", source)
	return nil
}
