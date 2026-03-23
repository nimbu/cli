package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogsCountCmd gets blog count.
type BlogsCountCmd struct{}

// Run executes the count command.
func (c *BlogsCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	count, err := api.Count(ctx, client, "/blogs/count")
	if err != nil {
		return fmt.Errorf("count blogs: %w", err)
	}

	return output.Print(ctx, output.CountPayload(count), []any{count}, func() error {
		_, err := output.Fprintf(ctx, "Blogs: %d\n", count)
		return err
	})
}
