package cmd

import (
	"context"
	"fmt"
	"strings"

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
	if err := requireScopes(ctx, client, []string{"read_content"}, "Example: nimbu auth scopes"); err != nil {
		return err
	}

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list blogs: %w", err)
	}

	var blogs []api.Blog
	var meta listFooterMeta

	if c.All {
		blogs, err = api.List[api.Blog](ctx, client, "/blogs", opts...)
		if err != nil {
			return fmt.Errorf("list blogs: %w", err)
		}
		meta = allListFooterMeta(len(blogs))
	} else {
		paged, err := api.ListPage[api.Blog](ctx, client, "/blogs", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list blogs: %w", err)
		}
		blogs = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(blogs))
		meta.probeTotal(ctx, client, "/blogs/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, blogs)
	}

	displayBlogs := buildBlogListRows(blogs)

	plainFields := []string{"id", "handle", "name"}
	tableFields := []string{"id", "handle", "name"}
	tableHeaders := []string{"ID", "HANDLE", "NAME"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, displayBlogs, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, displayBlogs, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "blogs", meta)
}

func buildBlogListRows(blogs []api.Blog) []api.Blog {
	rows := make([]api.Blog, len(blogs))
	for i := range blogs {
		blog := blogs[i]
		blog.Handle = blogDisplayHandle(blog)
		rows[i] = blog
	}
	return rows
}

func blogDisplayHandle(blog api.Blog) string {
	if strings.TrimSpace(blog.Handle) != "" {
		return blog.Handle
	}
	if strings.TrimSpace(blog.ID) != "" {
		return blog.ID
	}
	return "-"
}
