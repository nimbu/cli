package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// MenusListCmd lists menus.
type MenusListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *MenusListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
		return fmt.Errorf("list menus: %w", err)
	}

	var menus []api.MenuSummary
	var meta listFooterMeta

	if c.All {
		menus, err = api.List[api.MenuSummary](ctx, client, "/menus", opts...)
		if err != nil {
			return fmt.Errorf("list menus: %w", err)
		}
		meta = allListFooterMeta(len(menus))
	} else {
		paged, err := api.ListPage[api.MenuSummary](ctx, client, "/menus", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list menus: %w", err)
		}
		menus = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(menus))
		meta.probeTotal(ctx, client, "/menus/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, menus)
	}

	plainFields := []string{"id", "handle", "name"}
	tableFields := []string{"id", "handle", "name"}
	tableHeaders := []string{"ID", "HANDLE", "NAME"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, menus, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, menus, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "menus", meta)
}
