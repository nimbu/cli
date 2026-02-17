package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// UploadsListCmd lists uploads.
type UploadsListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
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

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list uploads: %w", err)
	}

	var uploads []api.Upload

	if c.All {
		uploads, err = api.List[api.Upload](ctx, client, "/uploads", opts...)
		if err != nil {
			return fmt.Errorf("list uploads: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Upload](ctx, client, "/uploads", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list uploads: %w", err)
		}
		uploads = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, uploads)
	}

	plainFields := []string{"id", "name", "url", "size"}
	tableFields := []string{"id", "name", "size", "mime_type"}
	tableHeaders := []string{"ID", "NAME", "SIZE", "MIME TYPE"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, uploads, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, uploads, fields, headers)
}
