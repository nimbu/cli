package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogsListCmd lists blogs.
type BlogsListCmd struct{}

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

	blogs, err := api.List[api.Blog](ctx, client, "/blogs")
	if err != nil {
		return fmt.Errorf("list blogs: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, blogs)
	}

	if mode.Plain {
		for _, b := range blogs {
			if err := output.Plain(ctx, b.ID, b.Handle, b.Name); err != nil {
				return err
			}
		}
		return nil
	}

	fields := []string{"id", "handle", "name"}
	headers := []string{"ID", "HANDLE", "NAME"}
	return output.WriteTable(ctx, blogs, fields, headers)
}
