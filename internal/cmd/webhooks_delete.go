package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// WebhooksDeleteCmd deletes a webhook.
type WebhooksDeleteCmd struct {
	ID string `arg:"" help:"Webhook ID"`
}

// Run executes the delete command.
func (c *WebhooksDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete webhook"); err != nil {
		return err
	}

	if err := requireForce(flags, "webhook "+c.ID); err != nil {
		return err
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/webhooks/" + url.PathEscape(c.ID)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("deleted"), []any{c.ID, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted webhook %s\n", c.ID)
		return err
	})
}
