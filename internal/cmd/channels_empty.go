package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type ChannelsEmptyCmd struct {
	Channel string `required:"" help:"Channel ID or slug"`
	Confirm string `help:"Exact channel slug confirmation"`
}

func (c *ChannelsEmptyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "empty channel"); err != nil {
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

	channel := api.Channel{Slug: c.Channel}
	if looksLikeObjectID(c.Channel) {
		path := "/channels/" + url.PathEscape(c.Channel)
		if err := client.Get(ctx, path, &channel); err != nil {
			var apiErr *api.Error
			if !errors.As(err, &apiErr) || !apiErr.IsNotFound() {
				return fmt.Errorf("resolve channel: %w", err)
			}
			channel = api.Channel{Slug: c.Channel}
		}
	}
	if err := requireExactConfirm(c.Confirm, channel.Slug, "empty channel "+channel.Slug); err != nil {
		return err
	}
	var result api.ActionStatus
	path := "/channels/" + url.PathEscape(c.Channel) + "/empty"
	if err := client.Post(ctx, path, map[string]any{"confirm": channel.Slug}, &result); err != nil {
		return fmt.Errorf("empty channel: %w", err)
	}
	return output.Print(ctx, result, []any{channel.Slug, result.Status, result.Message}, func() error {
		_, err := output.Fprintf(ctx, "Scheduled empty for channel %s\n", channel.Slug)
		return err
	})
}
