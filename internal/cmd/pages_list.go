package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PagesListCmd lists pages.
type PagesListCmd struct {
	All     bool   `help:"Fetch all pages"`
	Page    int    `help:"Page number" default:"1"`
	PerPage int    `help:"Items per page" default:"25"`
	Locale  string `help:"Filter by locale"`
}

// Run executes the list command.
func (c *PagesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithLocale(c.Locale))
	}

	var pages []api.Page

	if c.All {
		pages, err = api.List[api.Page](ctx, client, "/pages", opts...)
		if err != nil {
			return fmt.Errorf("list pages: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Page](ctx, client, "/pages", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list pages: %w", err)
		}
		pages = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, pages)
	}

	if mode.Plain {
		for _, p := range pages {
			if err := output.Plain(ctx, p.ID, p.Slug, p.Title); err != nil {
				return err
			}
		}
		return nil
	}

	fields := []string{"id", "slug", "title", "template", "published"}
	headers := []string{"ID", "SLUG", "TITLE", "TEMPLATE", "PUBLISHED"}
	return output.WriteTable(ctx, pages, fields, headers)
}
