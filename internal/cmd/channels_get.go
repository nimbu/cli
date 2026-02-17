package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsGetCmd gets channel details.
type ChannelsGetCmd struct {
	Channel string `arg:"" help:"Channel ID or slug"`
}

// Run executes the get command.
func (c *ChannelsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var ch api.Channel
	if err := client.Get(ctx, "/channels/"+url.PathEscape(c.Channel), &ch); err != nil {
		return fmt.Errorf("get channel: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, ch)
	}

	if mode.Plain {
		return output.Plain(ctx, ch.ID, ch.Slug, ch.Name, ch.EntryCount)
	}

	fmt.Printf("ID:          %s\n", ch.ID)
	fmt.Printf("Slug:        %s\n", ch.Slug)
	fmt.Printf("Name:        %s\n", ch.Name)
	if ch.Description != "" {
		fmt.Printf("Description: %s\n", ch.Description)
	}
	fmt.Printf("Entries:     %d\n", ch.EntryCount)

	return nil
}
