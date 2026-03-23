package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CollectionsCountCmd gets count of collections.
type CollectionsCountCmd struct{}

// Run executes the count command.
func (c *CollectionsCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	count, err := api.Count(ctx, client, "/collections/count")
	if err != nil {
		return fmt.Errorf("count collections: %w", err)
	}

	return output.Print(ctx, output.CountPayload(count), []any{count}, func() error {
		_, err := output.Fprintf(ctx, "Collections: %d\n", count)
		return err
	})
}
