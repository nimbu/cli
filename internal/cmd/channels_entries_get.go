package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesGetCmd gets an entry by ID or slug.
type ChannelEntriesGetCmd struct {
	Channel string `arg:"" help:"Channel ID or slug"`
	Entry   string `arg:"" help:"Entry ID or slug"`
	Locale  string `help:"Locale to fetch"`
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

	path := "/channels/" + c.Channel + "/entries/" + c.Entry
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithLocale(c.Locale))
	}

	var entry api.Entry
	if err := client.Get(ctx, path, &entry, opts...); err != nil {
		return fmt.Errorf("get entry: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, entry)
	}

	if mode.Plain {
		return output.Plain(ctx, entry.ID, entry.Slug, entry.Title, entry.Published)
	}

	fmt.Printf("ID:        %s\n", entry.ID)
	fmt.Printf("Slug:      %s\n", entry.Slug)
	fmt.Printf("Title:     %s\n", entry.Title)
	fmt.Printf("Published: %v\n", entry.Published)
	if entry.Locale != "" {
		fmt.Printf("Locale:    %s\n", entry.Locale)
	}
	if entry.Body != "" {
		fmt.Printf("Body:      %s\n", entry.Body)
	}

	return nil
}
