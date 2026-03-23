package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// UploadsListCmd lists uploads.
type UploadsListCmd struct {
	QueryFlags `embed:""`
	All        bool `help:"Fetch all pages"`
	Page       int  `help:"Page number" default:"1"`
	PerPage    int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *UploadsListCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	opts, err := listRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list uploads: %w", err)
	}

	var uploads []api.Upload
	var meta listFooterMeta

	if c.All {
		uploads, err = api.List[api.Upload](ctx, client, "/uploads", opts...)
		if err != nil {
			return fmt.Errorf("list uploads: %w", err)
		}
		meta = allListFooterMeta(len(uploads))
	} else {
		paged, err := api.ListPage[api.Upload](ctx, client, "/uploads", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list uploads: %w", err)
		}
		uploads = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(uploads))
		meta.probeTotal(ctx, client, "/uploads/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, uploads)
	}

	plainFields := []string{"id", "name", "url", "size"}
	tableFields := []string{"id", "name", "url", "size", "mime_type"}
	tableHeaders := []string{"ID", "NAME", "URL", "SIZE", "TYPE"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, uploads, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, uploads, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "uploads", meta)
}
