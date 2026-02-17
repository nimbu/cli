package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesCreateCmd creates a channel entry.
type ChannelEntriesCreateCmd struct {
	Channel     string   `arg:"" help:"Channel ID or slug"`
	File        string   `help:"Read entry JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. title=Hello, fields.teaser=Text)"`
}

// Run executes the create command.
func (c *ChannelEntriesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create entry"); err != nil {
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

	body, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		return err
	}

	path := "/channels/" + url.PathEscape(c.Channel) + "/entries"
	var entry api.Entry
	if err := client.Post(ctx, path, body, &entry); err != nil {
		return fmt.Errorf("create entry: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, entry)
	}

	if mode.Plain {
		return output.Plain(ctx, entry.ID)
	}

	fmt.Printf("Created entry %s\n", entry.ID)
	return nil
}
