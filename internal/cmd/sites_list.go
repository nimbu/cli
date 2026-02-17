package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// SitesListCmd lists accessible sites.
type SitesListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *SitesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}

	var sites []api.Site

	if c.All {
		sites, err = api.List[api.Site](ctx, client, "/sites", opts...)
		if err != nil {
			return fmt.Errorf("list sites: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Site](ctx, client, "/sites", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list sites: %w", err)
		}
		sites = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, sites)
	}

	plainFields := []string{"id", "subdomain", "name"}
	tableFields := []string{"id", "subdomain", "name"}
	tableHeaders := []string{"ID", "SUBDOMAIN", "NAME"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, sites, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, sites, fields, headers)
}
