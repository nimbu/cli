package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesGetCmd gets an entry by ID or slug.
type ChannelEntriesGetCmd struct {
	Channel string `arg:"" help:"Channel ID or slug"`
	Entry   string `arg:"" help:"Entry ID or slug"`
}

// Run executes the get command.
func (c *ChannelEntriesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/channels/" + url.PathEscape(c.Channel) + "/entries/" + url.PathEscape(c.Entry)
	var opts []api.RequestOption
	if flags.Locale != "" {
		opts = append(opts, api.WithLocale(flags.Locale))
	}

	var entry api.Entry
	if err := client.Get(ctx, path, &entry, opts...); err != nil {
		return fmt.Errorf("get entry: %w", err)
	}

	return output.Detail(ctx, entry, []any{entry.ID, entry.Slug, entry.Title, entry.Published}, []output.Field{
		output.FAlways("ID", entry.ID),
		output.FAlways("Slug", entry.Slug),
		output.FAlways("Title", entry.Title),
		output.FAlways("Published", entry.Published),
		output.F("Locale", entry.Locale),
		output.F("Body", entry.Body),
	})
}
