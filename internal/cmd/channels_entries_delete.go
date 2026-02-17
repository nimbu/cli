package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesDeleteCmd deletes a channel entry.
type ChannelEntriesDeleteCmd struct {
	Channel string `arg:"" help:"Channel ID or slug"`
	Entry   string `arg:"" help:"Entry ID or slug"`
}

// Run executes the delete command.
func (c *ChannelEntriesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete entry"); err != nil {
		return err
	}

	if err := requireForce(flags, "entry "+c.Entry); err != nil {
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

	path := "/channels/" + url.PathEscape(c.Channel) + "/entries/" + url.PathEscape(c.Entry)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete entry: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("entry deleted"))
	}

	if mode.Plain {
		return output.Plain(ctx, c.Entry, "deleted")
	}

	fmt.Printf("Deleted entry %s\n", c.Entry)
	return nil
}
