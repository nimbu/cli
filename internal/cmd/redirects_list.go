package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// RedirectsListCmd lists redirects.
type RedirectsListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *RedirectsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list redirects: %w", err)
	}

	var redirects []api.Redirect
	if c.All {
		redirects, err = api.List[api.Redirect](ctx, client, "/redirects", opts...)
		if err != nil {
			return fmt.Errorf("list redirects: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Redirect](ctx, client, "/redirects", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list redirects: %w", err)
		}
		redirects = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, redirects)
	}

	plainFields := []string{"id", "source", "target"}
	tableFields := []string{"id", "source", "target"}
	tableHeaders := []string{"ID", "SOURCE", "TARGET"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, redirects, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, redirects, fields, headers)
}
