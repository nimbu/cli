package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesGetCmd gets an entry by ID or slug.
type ChannelEntriesGetCmd struct {
	QueryFlags `embed:""`
	Channel    string `required:"" help:"Channel ID or slug"`
	Entry      string `required:"" help:"Entry ID or slug"`
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
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}

	var entry api.Entry
	if err := client.Get(ctx, path, &entry, opts...); err != nil {
		if api.IsNotFound(err) {
			found, findErr := findChannelEntryBySlug(ctx, client, c.Channel, c.Entry, opts...)
			if findErr != nil {
				return fmt.Errorf("get entry: %w", findErr)
			}
			if found.ID != "" {
				entry = found
			} else {
				return fmt.Errorf("get entry: %w", err)
			}
		} else {
			return fmt.Errorf("get entry: %w", err)
		}
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

func findChannelEntryBySlug(ctx context.Context, client *api.Client, channel, slug string, opts ...api.RequestOption) (api.Entry, error) {
	escapedSlug := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(slug)
	where := fmt.Sprintf(`_slug:"%s"`, escapedSlug)
	requestOpts := append([]api.RequestOption{api.WithParam("where", where)}, opts...)
	path := "/channels/" + url.PathEscape(channel) + "/entries"
	var entries []api.Entry
	if err := client.Get(ctx, path, &entries, requestOpts...); err != nil {
		return api.Entry{}, err
	}
	if len(entries) == 0 {
		return api.Entry{}, nil
	}
	return entries[0], nil
}
