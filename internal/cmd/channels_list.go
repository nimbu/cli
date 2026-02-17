package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsListCmd lists channels.
type ChannelsListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *ChannelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}

	var channels []api.Channel

	if c.All {
		channels, err = api.List[api.Channel](ctx, client, "/channels", opts...)
		if err != nil {
			return fmt.Errorf("list channels: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Channel](ctx, client, "/channels", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list channels: %w", err)
		}
		channels = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, channels)
	}

	plainFields := []string{"id", "slug", "name"}
	tableFields := []string{"id", "slug", "name", "entry_count"}
	tableHeaders := []string{"ID", "SLUG", "NAME", "ENTRIES"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, channels, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, channels, fields, headers)
}
