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
	Channel string `arg:"" help:"Channel ID or slug"`
	All     bool   `help:"Fetch all pages"`
	Page    int    `help:"Page number" default:"1"`
	PerPage int    `help:"Items per page" default:"25"`
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
	if err := requireScopes(ctx, client, []string{"read_channels"}, "Example: nimbu-cli auth scopes"); err != nil {
		return err
	}

	path := "/channels/" + url.PathEscape(c.Channel) + "/entries"
	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list entries: %w", err)
	}

	var entries []api.Entry
	var meta listFooterMeta

	if c.All {
		entries, err = api.List[api.Entry](ctx, client, path, opts...)
		if err != nil {
			return fmt.Errorf("list entries: %w", err)
		}
		meta = allListFooterMeta(len(entries))
	} else {
		paged, err := api.ListPage[api.Entry](ctx, client, path, c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list entries: %w", err)
		}
		entries = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(entries))
		meta.probeTotal(ctx, client, "/channels/"+url.PathEscape(c.Channel)+"/entries/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, entries)
	}

	displayEntries := buildChannelEntryListRows(entries)

	plainFields := []string{"id", "slug", "title"}
	tableFields := []string{"id", "slug", "title", "published"}
	tableHeaders := []string{"ID", "SLUG", "TITLE", "PUBLISHED"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, displayEntries, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, displayEntries, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "entries", meta)
}

func buildChannelEntryListRows(entries []api.Entry) []api.Entry {
	rows := make([]api.Entry, len(entries))
	for i := range entries {
		entry := entries[i]
		entry.Title = entryDisplayTitle(entry)
		rows[i] = entry
	}
	return rows
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

	if strings.TrimSpace(entry.Slug) != "" {
		return entry.Slug
	}

	return entry.ID
}
