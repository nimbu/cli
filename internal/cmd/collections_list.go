package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CollectionsListCmd lists collections.
type CollectionsListCmd struct {
	QueryFlags `embed:""`
	All        bool `help:"Fetch all pages"`
	Page       int  `help:"Page number" default:"1"`
	PerPage    int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *CollectionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
		return fmt.Errorf("list collections: %w", err)
	}

	var collections []api.Collection
	var meta listFooterMeta
	if c.All {
		collections, err = api.List[api.Collection](ctx, client, "/collections", opts...)
		if err != nil {
			return fmt.Errorf("list collections: %w", err)
		}
		meta = allListFooterMeta(len(collections))
	} else {
		paged, err := api.ListPage[api.Collection](ctx, client, "/collections", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list collections: %w", err)
		}
		collections = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(collections))
		meta.probeTotal(ctx, client, "/collections/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, collections)
	}

	plainFields := []string{"id", "slug", "name", "status", "type"}
	tableFields := []string{"id", "slug", "name", "status", "type", "product_count"}
	tableHeaders := []string{"ID", "SLUG", "NAME", "STATUS", "TYPE", "PRODUCTS"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, collections, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, collections, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "collections", meta)
}
