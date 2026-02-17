package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/output"
)

// WebhooksDeleteCmd deletes a webhook.
type WebhooksDeleteCmd struct {
	ID string `arg:"" help:"Webhook ID"`
}

// Run executes the delete command.
func (c *WebhooksDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("cannot delete webhook in readonly mode")
	}

	if !flags.Force {
		return fmt.Errorf("use --force to confirm deletion")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	if err := client.Delete(ctx, "/webhooks/"+c.ID, nil); err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("deleted"))
	}

	if mode.Plain {
		return output.Plain(ctx, c.ID, "deleted")
	}

	fmt.Printf("Deleted webhook %s\n", c.ID)
	return nil
}
