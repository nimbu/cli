package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// MenusListCmd lists menus.
type MenusListCmd struct {
	QueryFlags `embed:""`
	All        bool `help:"Fetch all pages"`
	Page       int  `help:"Page number" default:"1"`
	PerPage    int  `help:"Items per page" default:"25"`
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
	if err := requireScopes(ctx, client, []string{"read_content"}, "Example: nimbu auth scopes"); err != nil {
		return err
	}

	opts, err := localizedContentListRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list menus: %w", err)
	}

	var documents []api.Document[api.MenuSummary]
	var meta listFooterMeta

	if c.All {
		documents, err = api.List[api.Document[api.MenuSummary]](ctx, client, "/menus", opts...)
		if err != nil {
			return fmt.Errorf("list menus: %w", err)
		}
		meta = allListFooterMeta(len(documents))
	} else {
		paged, err := api.ListPage[api.Document[api.MenuSummary]](ctx, client, "/menus", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list menus: %w", err)
		}
		documents = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(documents))
		meta.probeTotal(ctx, client, "/menus/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, documents)
	}
	menus, err := localizedDocumentMaps(documents, c.Locale)
	if err != nil {
		return fmt.Errorf("project menu locale: %w", err)
	}
	for _, menu := range menus {
		if handle, _ := menu["handle"].(string); strings.TrimSpace(handle) == "" {
			menu["handle"] = menu["slug"]
		}
	}

	plainFields := []string{"id", "handle", "name"}
	tableFields := []string{"id", "handle", "name"}
	tableHeaders := []string{"ID", "HANDLE", "NAME"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, menus, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, menus, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "menus", meta)
}
