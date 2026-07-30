package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesListCmd lists channel entries.
type ChannelEntriesListCmd struct {
	QueryFlags `embed:""`
	Channel    string `required:"" help:"Channel ID or slug"`
	All        bool   `help:"Fetch all pages"`
	Page       int    `help:"Page number" default:"1"`
	PerPage    int    `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *ChannelEntriesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	if err := requireScopes(ctx, client, []string{"read_channels"}, "Example: nimbu auth scopes"); err != nil {
		return err
	}

	path := "/channels/" + url.PathEscape(c.Channel) + "/entries"
	opts, err := channelEntryListRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list entries: %w", err)
	}

	var documents []api.Document[api.Entry]
	var meta listFooterMeta

	if c.All {
		documents, err = api.List[api.Document[api.Entry]](ctx, client, path, opts...)
		if err != nil {
			return fmt.Errorf("list entries: %w", err)
		}
		meta = allListFooterMeta(len(documents))
	} else {
		paged, err := api.ListPage[api.Document[api.Entry]](ctx, client, path, c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list entries: %w", err)
		}
		documents = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(documents))
		meta.probeTotal(ctx, client, "/channels/"+url.PathEscape(c.Channel)+"/entries/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		if documents == nil {
			documents = []api.Document[api.Entry]{}
		}
		return output.JSON(ctx, documents)
	}

	displayEntries, err := localizedDocumentMaps(documents, c.Locale)
	if err != nil {
		return fmt.Errorf("project entry locale: %w", err)
	}
	for _, entry := range displayEntries {
		title, _ := entry["title"].(string)
		if strings.TrimSpace(title) == "" {
			if fields, ok := entry["fields"].(map[string]any); ok {
				title, _ = fields["title"].(string)
			}
		}
		if strings.TrimSpace(title) == "" {
			for _, key := range []string{"title_field_value", "name", "slug", "id"} {
				if candidate, _ := entry[key].(string); strings.TrimSpace(candidate) != "" {
					title = candidate
					break
				}
			}
		}
		entry["title"] = title
	}

	plainFields := []string{"id", "slug", "title"}
	tableFields := []string{"id", "slug", "title", "published"}
	tableHeaders := []string{"ID", "SLUG", "TITLE", "PUBLISHED"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, displayEntries, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, displayEntries, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "entries", meta)
}

func entryDisplayTitle(entry api.Entry) string {
	if strings.TrimSpace(entry.Title) != "" {
		return entry.Title
	}

	if entry.Fields != nil {
		if raw, ok := entry.Fields["title"]; ok {
			if title, ok := raw.(string); ok {
				title = strings.TrimSpace(title)
				if title != "" {
					return title
				}
			}
		}
	}
	if entry.Extra != nil {
		for _, key := range []string{"title", "title_field_value", "name"} {
			if title := extraString(entry.Extra, key); title != "" {
				return title
			}
		}
	}

	if strings.TrimSpace(entry.Slug) != "" {
		return entry.Slug
	}

	return entry.ID
}

func extraString(values map[string]any, key string) string {
	raw, ok := values[key]
	if !ok {
		return ""
	}
	text, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func channelEntryListRequestOptions(flags *QueryFlags, extra ...api.RequestOption) ([]api.RequestOption, error) {
	if flags == nil {
		return listRequestOptions(flags, extra...)
	}
	requestFlags := *flags
	locale := requestFlags.Locale
	requestFlags.Locale = ""
	opts, err := listRequestOptions(&requestFlags, extra...)
	if err != nil {
		return nil, err
	}
	if locale != "" {
		opts = append(opts, api.WithContentLocale(locale))
	}
	return opts, nil
}
