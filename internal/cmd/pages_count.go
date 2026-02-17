package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PagesCountCmd gets page count.
type PagesCountCmd struct{}

// Run executes the count command.
func (c *PagesCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	count, err := api.Count(ctx, client, "/pages/count")
	if err != nil {
		return fmt.Errorf("count pages: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.CountPayload(count))
	}

	if mode.Plain {
		return output.Plain(ctx, count)
	}

	fmt.Printf("Pages: %d\n", count)
	return nil
}
