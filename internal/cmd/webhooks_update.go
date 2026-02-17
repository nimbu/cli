package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// WebhooksUpdateCmd updates a webhook.
type WebhooksUpdateCmd struct {
	ID          string   `arg:"" help:"Webhook ID"`
	File        string   `help:"Read webhook data from file (use - for stdin)" short:"f"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. url=https://example.com, active:=true)"`
}

// Run executes the update command.
func (c *WebhooksUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update webhook"); err != nil {
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

	data, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		return err
	}

	var webhook api.Webhook
	path := "/webhooks/" + url.PathEscape(c.ID)
	if err := client.Put(ctx, path, data, &webhook); err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, webhook)
	}

	if mode.Plain {
		return output.Plain(ctx, webhook.ID)
	}

	fmt.Printf("Updated webhook %s\n", webhook.ID)
	return nil
}
