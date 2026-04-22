package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesCountCmd counts channel entries.
type ChannelEntriesCountCmd struct {
	QueryFlags `embed:""`
	Channel    string `arg:"" help:"Channel ID or slug"`
}

// Run executes the count command.
func (c *ChannelEntriesCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/channels/" + url.PathEscape(c.Channel) + "/entries/count"
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}

	count, err := api.Count(ctx, client, path, opts...)
	if err != nil {
		return fmt.Errorf("count entries: %w", err)
	}

	return output.Print(ctx, output.CountPayload(count), []any{count}, func() error {
		_, err := output.Fprintf(ctx, "Count: %d\n", count)
		return err
	})
}
