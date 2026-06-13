package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsDeleteCmd deletes a channel.
type ChannelsDeleteCmd struct {
	Channel string `required:"" help:"Channel ID or slug"`
}

// Run executes the delete command.
func (c *ChannelsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete channel"); err != nil {
		return err
	}

	if err := requireForce(flags, "channel "+c.Channel); err != nil {
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

	if err := api.DeleteChannel(ctx, client, c.Channel); err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("channel deleted"), []any{c.Channel, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted channel %s\n", c.Channel)
		return err
	})
}
