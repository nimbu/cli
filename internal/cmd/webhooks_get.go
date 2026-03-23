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

	var events string
	if len(webhook.Events) > 0 {
		events = fmt.Sprintf("%v", webhook.Events)
	}

	return output.Detail(ctx, webhook, []any{webhook.ID, webhook.URL}, []output.Field{
		output.FAlways("ID", webhook.ID),
		output.FAlways("URL", webhook.URL),
		output.F("Events", events),
	})
}
