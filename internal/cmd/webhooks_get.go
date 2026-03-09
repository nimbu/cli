package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// WebhooksGetCmd gets webhook details.
type WebhooksGetCmd struct {
	ID string `arg:"" help:"Webhook ID"`
}

// Run executes the get command.
func (c *WebhooksGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var webhook api.Webhook
	path := "/webhooks/" + url.PathEscape(c.ID)
	if err := client.Get(ctx, path, &webhook); err != nil {
		return fmt.Errorf("get webhook: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, webhook)
	}

	if mode.Plain {
		return output.Plain(ctx, webhook.ID, webhook.URL)
	}

	fmt.Printf("ID:     %s\n", webhook.ID)
	fmt.Printf("URL:    %s\n", webhook.URL)
	if len(webhook.Events) > 0 {
		fmt.Printf("Events: %v\n", webhook.Events)
	}

	return nil
}
