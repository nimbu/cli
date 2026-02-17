package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// SitesListCmd lists accessible sites.
type SitesListCmd struct{}

// Run executes the list command.
func (c *SitesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	sites, err := api.List[api.Site](ctx, client, "/sites")
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, sites)
	}

	if mode.Plain {
		for _, s := range sites {
			if err := output.Plain(ctx, s.ID, s.Subdomain, s.Name); err != nil {
				return err
			}
		}
		return nil
	}

	// Human-readable table
	fields := []string{"id", "subdomain", "name"}
	headers := []string{"ID", "SUBDOMAIN", "NAME"}
	return output.WriteTable(ctx, sites, fields, headers)
}
