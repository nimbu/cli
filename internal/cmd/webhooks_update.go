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

// WebhooksUpdateCmd updates a webhook.
type WebhooksUpdateCmd struct {
	ID   string `arg:"" help:"Webhook ID"`
	File string `help:"Read webhook data from file" short:"f" type:"existingfile"`
}

// Run executes the update command.
func (c *WebhooksUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("cannot update webhook in readonly mode")
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
	if err := client.Put(ctx, "/webhooks/"+c.ID, data, &webhook); err != nil {
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
