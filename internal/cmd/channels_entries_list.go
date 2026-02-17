package cmd

import (
	"context"
	"fmt"
	"net/url"

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

	path := "/channels/" + url.PathEscape(c.Channel) + "/entries"
	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list entries: %w", err)
	}

	var entries []api.Entry

	if c.All {
		entries, err = api.List[api.Entry](ctx, client, path, opts...)
		if err != nil {
			return fmt.Errorf("list entries: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Entry](ctx, client, path, c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list entries: %w", err)
		}
		entries = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, entries)
	}

	plainFields := []string{"id", "slug", "title"}
	tableFields := []string{"id", "slug", "title", "published"}
	tableHeaders := []string{"ID", "SLUG", "TITLE", "PUBLISHED"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, entries, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, entries, fields, headers)
}
