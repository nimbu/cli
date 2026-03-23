package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// WebhooksCreateCmd creates a webhook.
type WebhooksCreateCmd struct {
	File        string   `help:"Read webhook data from file (use - for stdin)" short:"f"`
	URL         string   `help:"Webhook URL"`
	Events      string   `help:"Comma-separated event types"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. url=https://example.com, events:=[\"order.created\"])"`
}

// Run executes the create command.
func (c *WebhooksCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create webhook"); err != nil {
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

	flagData := map[string]any{}
	if c.URL != "" {
		flagData["url"] = c.URL
	}
	if c.Events != "" {
		events := make([]string, 0)
		for _, raw := range strings.Split(c.Events, ",") {
			event := strings.TrimSpace(raw)
			if event != "" {
				events = append(events, event)
			}
		}
		flagData["events"] = events
	}

	var data map[string]any
	switch {
	case c.File != "" || len(c.Assignments) > 0:
		data, err = readJSONBodyInput(c.File, c.Assignments)
		if err != nil {
			return err
		}
	case len(flagData) > 0:
		data = map[string]any{}
	default:
		data, err = readJSONBodyInput("", nil)
		if err != nil {
			return err
		}
	}

	if len(flagData) > 0 {
		data, err = mergeJSONBodies(data, flagData)
		if err != nil {
			return fmt.Errorf("merge webhook flags with request body: %w", err)
		}
	}

	var webhook api.Webhook
	if err := client.Post(ctx, "/webhooks", data, &webhook); err != nil {
		return fmt.Errorf("create webhook: %w", err)
	}

	return output.Print(ctx, webhook, []any{webhook.ID}, func() error {
		_, err := output.Fprintf(ctx, "Created webhook %s\n", webhook.ID)
		return err
	})
}
