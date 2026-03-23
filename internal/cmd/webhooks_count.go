package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// WebhooksCountCmd gets webhook count.
type WebhooksCountCmd struct{}

// Run executes the count command.
func (c *WebhooksCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	count, err := api.Count(ctx, client, "/webhooks/count")
	if err != nil {
		return fmt.Errorf("count webhooks: %w", err)
	}

	return output.Print(ctx, output.CountPayload(count), []any{count}, func() error {
		_, err := output.Fprintf(ctx, "Webhooks: %d\n", count)
		return err
	})
}
