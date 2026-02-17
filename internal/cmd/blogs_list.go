package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogsListCmd lists blogs.
type BlogsListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *BlogsListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
		return fmt.Errorf("list blogs: %w", err)
	}

	var blogs []api.Blog

	if c.All {
		blogs, err = api.List[api.Blog](ctx, client, "/blogs", opts...)
		if err != nil {
			return fmt.Errorf("list blogs: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Blog](ctx, client, "/blogs", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list blogs: %w", err)
		}
		blogs = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, blogs)
	}

	plainFields := []string{"id", "handle", "name"}
	tableFields := []string{"id", "handle", "name"}
	tableHeaders := []string{"ID", "HANDLE", "NAME"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, blogs, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, blogs, fields, headers)
}
