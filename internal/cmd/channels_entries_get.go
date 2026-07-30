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

	var document api.Document[api.Entry]
	if err := client.Get(ctx, path, &document, opts...); err != nil {
		if api.IsNotFound(err) {
			found, findErr := findChannelEntryDocumentBySlug(ctx, client, c.Channel, c.Entry, opts...)
			if findErr != nil {
				return fmt.Errorf("get entry: %w", findErr)
			}
			if found.Value.ID != "" {
				document = found
			} else {
				return fmt.Errorf("get entry: %w", err)
			}
		} else {
			return fmt.Errorf("get entry: %w", err)
		}
	}
	projected, err := output.ProjectLocale(document, c.Locale)
	if err != nil {
		return fmt.Errorf("project entry locale: %w", err)
	}
	display := projected.(map[string]any)

	return output.Detail(ctx, document, []any{display["id"], display["slug"], display["title"], display["published"]}, []output.Field{
		output.FAlways("ID", display["id"]),
		output.FAlways("Slug", display["slug"]),
		output.FAlways("Title", display["title"]),
		output.FAlways("Published", display["published"]),
		output.F("Locale", display["locale"]),
		output.F("Body", display["body"]),
	})
}

func findChannelEntryBySlug(ctx context.Context, client *api.Client, channel, slug string, opts ...api.RequestOption) (api.Entry, error) {
	document, err := findChannelEntryDocumentBySlug(ctx, client, channel, slug, opts...)
	return document.Value, err
}

func findChannelEntryDocumentBySlug(ctx context.Context, client *api.Client, channel, slug string, opts ...api.RequestOption) (api.Document[api.Entry], error) {
	escapedSlug := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(slug)
	where := fmt.Sprintf(`_slug:"%s"`, escapedSlug)
	requestOpts := append([]api.RequestOption{api.WithParam("where", where)}, opts...)
	path := "/channels/" + url.PathEscape(channel) + "/entries"
	var entries []api.Document[api.Entry]
	if err := client.Get(ctx, path, &entries, requestOpts...); err != nil {
		return api.Document[api.Entry]{}, err
	}
	if len(entries) == 0 {
		return api.Document[api.Entry]{}, nil
	}
	return entries[0], nil
}
