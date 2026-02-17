package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesDeleteCmd deletes a channel entry.
type ChannelEntriesDeleteCmd struct {
	Channel string `arg:"" help:"Channel ID or slug"`
	Entry   string `arg:"" help:"Entry ID or slug"`
}

// Run executes the delete command.
func (c *ChannelEntriesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("write operations disabled in readonly mode")
	}

	if !flags.Force {
		return fmt.Errorf("delete requires --force flag")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/channels/" + c.Channel + "/entries/" + c.Entry
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete entry: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("entry deleted"))
	}

	if mode.Plain {
		return output.Plain(ctx, "deleted", c.Entry)
	}

	fmt.Printf("Deleted entry %s\n", c.Entry)
	return nil
}
