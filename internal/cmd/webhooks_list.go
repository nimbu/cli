package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// WebhooksListCmd lists webhooks.
type WebhooksListCmd struct{}

// Run executes the list command.
func (c *WebhooksListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	webhooks, err := api.List[api.Webhook](ctx, client, "/webhooks")
	if err != nil {
		return fmt.Errorf("list webhooks: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, webhooks)
	}

	if mode.Plain {
		for _, w := range webhooks {
			if err := output.Plain(ctx, w.ID, w.URL, w.Active); err != nil {
				return err
			}
		}
		return nil
	}

	fields := []string{"id", "url", "active"}
	headers := []string{"ID", "URL", "ACTIVE"}
	return output.WriteTable(ctx, webhooks, fields, headers)
}
