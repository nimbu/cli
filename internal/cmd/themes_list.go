package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemesListCmd lists themes.
type ThemesListCmd struct {
	QueryFlags `embed:""`
	All        bool `help:"Fetch all pages"`
	Page       int  `help:"Page number" default:"1"`
	PerPage    int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *ThemesListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
		return fmt.Errorf("list themes: %w", err)
	}

	var themes []api.Theme

	if c.All {
		themes, err = api.List[api.Theme](ctx, client, "/themes", opts...)
		if err != nil {
			return fmt.Errorf("list themes: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Theme](ctx, client, "/themes", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list themes: %w", err)
		}
		themes = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, themes)
	}

	plainFields := []string{"id", "name", "active"}
	tableFields := []string{"id", "name", "active"}
	tableHeaders := []string{"ID", "NAME", "ACTIVE"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, themes, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	return output.WriteTable(ctx, themes, fields, headers)
}
