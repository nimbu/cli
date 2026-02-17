package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsListCmd lists channels.
type ChannelsListCmd struct{}

// Run executes the list command.
func (c *ChannelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	channels, err := api.List[api.Channel](ctx, client, "/channels")
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, channels)
	}

	if mode.Plain {
		for _, ch := range channels {
			if err := output.Plain(ctx, ch.ID, ch.Slug, ch.Name); err != nil {
				return err
			}
		}
		return nil
	}

	fields := []string{"id", "slug", "name", "entry_count"}
	headers := []string{"ID", "SLUG", "NAME", "ENTRIES"}
	return output.WriteTable(ctx, channels, fields, headers)
}
