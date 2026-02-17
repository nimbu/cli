package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// WebhooksCreateCmd creates a webhook.
type WebhooksCreateCmd struct {
	File   string `help:"Read webhook data from file" short:"f" type:"existingfile"`
	URL    string `help:"Webhook URL"`
	Events string `help:"Comma-separated event types"`
}

// Run executes the create command.
func (c *WebhooksCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("cannot create webhook in readonly mode")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	// Read webhook data
	var data map[string]any
	if c.File != "" {
		f, err := os.Open(c.File)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}
		defer func() { _ = f.Close() }()
		if err := json.NewDecoder(f).Decode(&data); err != nil {
			return fmt.Errorf("decode file: %w", err)
		}
	} else if c.URL != "" {
		data = map[string]any{"url": c.URL}
		if c.Events != "" {
			data["events"] = c.Events
		}
	} else {
		// Read from stdin
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		if err := json.Unmarshal(input, &data); err != nil {
			return fmt.Errorf("decode stdin: %w", err)
		}
	}

	var webhook api.Webhook
	if err := client.Post(ctx, "/webhooks", data, &webhook); err != nil {
		return fmt.Errorf("create webhook: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, webhook)
	}

	if mode.Plain {
		return output.Plain(ctx, webhook.ID)
	}

	fmt.Printf("Created webhook %s\n", webhook.ID)
	return nil
}
