package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesUpdateCmd updates a channel entry.
type ChannelEntriesUpdateCmd struct {
	Channel     string   `arg:"" help:"Channel ID or slug"`
	Entry       string   `arg:"" help:"Entry ID or slug"`
	File        string   `help:"Read entry JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. title=Hello, fields.teaser=Text)"`
}

// Run executes the update command.
func (c *ChannelEntriesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update entry"); err != nil {
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

	path := "/channels/" + url.PathEscape(c.Channel) + "/entries/" + url.PathEscape(c.Entry)
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
