package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// AppsListCmd lists apps.
type AppsListCmd struct {
	QueryFlags `embed:""`
	All        bool `help:"Fetch all pages"`
	Page       int  `help:"Page number" default:"1"`
	PerPage    int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *AppsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	opts, err := listRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list apps: %w", err)
	}

	var apps []api.App
	if c.All {
		apps, err = api.List[api.App](ctx, client, "/apps", opts...)
		if err != nil {
			return fmt.Errorf("list apps: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.App](ctx, client, "/apps", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list apps: %w", err)
		}
		apps = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, apps)
	}

	plainFields := []string{"key", "name", "domain", "callback_url"}
	tableFields := []string{"key", "name", "domain", "callback_url"}
	tableHeaders := []string{"KEY", "NAME", "DOMAIN", "CALLBACK"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, apps, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	return output.WriteTable(ctx, apps, fields, headers)
}
