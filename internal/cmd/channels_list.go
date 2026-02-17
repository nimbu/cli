package cmd

import (
	"context"
	"fmt"
	"net/url"
	"sync"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsListCmd lists channels.
type ChannelsListCmd struct {
	All            bool `help:"Fetch all pages"`
	Page           int  `help:"Page number" default:"1"`
	PerPage        int  `help:"Items per page" default:"25"`
	WithEntryCount bool `name:"with-entry-count" help:"Fetch entry count for each channel" default:"true"`
	NoEntryCount   bool `name:"no-entry-count" help:"Skip fetching entry count for each channel"`
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

	if c.WithEntryCount && !c.NoEntryCount {
		fetchChannelEntryCounts(ctx, client, channels)
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

func fetchChannelEntryCounts(ctx context.Context, client *api.Client, channels []api.Channel) {
	const workers = 6

	var wg sync.WaitGroup
	jobs := make(chan int)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				countPath := channels[idx].Slug
				if countPath == "" {
					countPath = channels[idx].ID
				}
				if countPath == "" {
					continue
				}

				count, err := api.Count(ctx, client, "/channels/"+url.PathEscape(countPath)+"/entries/count")
				if err != nil {
					continue
				}

				countCopy := count
				channels[idx].EntryCount = &countCopy
			}
		}()
	}

	for i := range channels {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}
