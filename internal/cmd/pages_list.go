package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PagesListCmd lists pages.
type PagesListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
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
	if err := requireScopes(ctx, client, []string{"read_content"}, "Example: nimbu-cli auth scopes"); err != nil {
		return err
	}

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list pages: %w", err)
	}

	var pages []api.PageSummary
	var meta listFooterMeta

	if c.All {
		pages, err = api.List[api.PageSummary](ctx, client, "/pages", opts...)
		if err != nil {
			return fmt.Errorf("list pages: %w", err)
		}
		meta = allListFooterMeta(len(pages))
	} else {
		paged, err := api.ListPage[api.PageSummary](ctx, client, "/pages", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list pages: %w", err)
		}
		pages = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(pages))
		meta.probeTotal(ctx, client, "/pages/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, pages)
	}

	plainFields := []string{"id", "fullpath", "title"}
	tableFields := []string{"id", "fullpath", "title", "template", "published"}
	tableHeaders := []string{"ID", "FULLPATH", "TITLE", "TEMPLATE", "PUBLISHED"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, pages, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, pages, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "pages", meta)
}
