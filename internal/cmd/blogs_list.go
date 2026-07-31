package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogsListCmd lists blogs.
type BlogsListCmd struct {
	QueryFlags `embed:""`
	All        bool `help:"Fetch all pages"`
	Page       int  `help:"Page number" default:"1"`
	PerPage    int  `help:"Items per page" default:"25"`
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
	if err := requireScopes(ctx, client, []string{"read_content"}, "Example: nimbu auth scopes"); err != nil {
		return err
	}

	opts, err := localizedContentListRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list blogs: %w", err)
	}

	var documents []api.Document[api.Blog]
	var meta listFooterMeta

	if c.All {
		documents, err = api.List[api.Document[api.Blog]](ctx, client, "/blogs", opts...)
		if err != nil {
			return fmt.Errorf("list blogs: %w", err)
		}
		meta = allListFooterMeta(len(documents))
	} else {
		paged, err := api.ListPage[api.Document[api.Blog]](ctx, client, "/blogs", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list blogs: %w", err)
		}
		documents = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(documents))
		meta.probeTotal(ctx, client, "/blogs/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, documents)
	}

	displayBlogs, err := localizedDocumentMaps(documents, c.Locale)
	if err != nil {
		return fmt.Errorf("project blog locale: %w", err)
	}
	for _, blog := range displayBlogs {
		applyBlogDisplayHandle(blog)
	}

	plainFields := []string{"id", "handle", "name"}
	tableFields := []string{"id", "handle", "name"}
	tableHeaders := []string{"ID", "HANDLE", "NAME"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, displayBlogs, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, displayBlogs, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "blogs", meta)
}
