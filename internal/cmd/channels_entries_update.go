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

// ChannelEntriesUpdateCmd updates a channel entry.
type ChannelEntriesUpdateCmd struct {
	Channel string `arg:"" help:"Channel ID or slug"`
	Entry   string `arg:"" help:"Entry ID or slug"`
	File    string `help:"JSON file path (default: stdin)" type:"existingfile"`
}

// Run executes the update command.
func (c *ChannelEntriesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("write operations disabled in readonly mode")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	// Read input
	var input io.Reader = os.Stdin
	if c.File != "" {
		f, err := os.Open(c.File)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}
		defer func() { _ = f.Close() }()
		input = f
	}

	data, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	path := "/channels/" + c.Channel + "/entries/" + c.Entry
	var entry api.Entry
	if err := client.Put(ctx, path, body, &entry); err != nil {
		return fmt.Errorf("update entry: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, entry)
	}

	if mode.Plain {
		return output.Plain(ctx, entry.ID)
	}

	fmt.Printf("Updated entry %s\n", entry.ID)
	return nil
}
