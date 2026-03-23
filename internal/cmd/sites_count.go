package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// SitesCountCmd gets site count.
type SitesCountCmd struct{}

// Run executes the count command.
func (c *SitesCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	count, err := api.Count(ctx, client, "/sites/count")
	if err != nil {
		return fmt.Errorf("count sites: %w", err)
	}

	return output.Print(ctx, output.CountPayload(count), []any{count}, func() error {
		_, err := output.Fprintf(ctx, "Sites: %d\n", count)
		return err
	})
}
