package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesCountCmd counts channel entries.
type ChannelEntriesCountCmd struct {
	Channel string `arg:"" help:"Channel ID or slug"`
	Locale  string `help:"Filter by locale"`
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

	path := "/channels/" + c.Channel + "/entries/count"
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithLocale(c.Locale))
	}

	count, err := api.Count(ctx, client, path, opts...)
	if err != nil {
		return fmt.Errorf("count entries: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.CountPayload(count))
	}

	if mode.Plain {
		return output.Plain(ctx, count)
	}

	fmt.Printf("Count: %d\n", count)
	return nil
}
