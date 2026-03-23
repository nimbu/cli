package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// ChannelEntriesCopyCmd copies channel entries between sites.
type ChannelEntriesCopyCmd struct {
	From          string `help:"Source site/channel" required:"" name:"from"`
	To            string `help:"Target site/channel" required:"" name:"to"`
	FromHost      string `help:"Source API base URL or host" name:"from-host"`
	ToHost        string `help:"Target API base URL or host" name:"to-host"`
	Recursive     bool   `help:"Recursively copy referenced channel entries"`
	Only          string `help:"Comma-separated channel allowlist when using --recursive"`
	Query         string `help:"Raw query string to append to the source entry list"`
	Where         string `help:"Where expression for source entry selection"`
	PerPage       int    `help:"Items per page" name:"per-page"`
	Upsert        string `help:"Comma-separated upsert fields, optionally channel-scoped as channel:field"`
	CopyCustomers bool   `name:"copy-customers" help:"Copy related customers for owners and customer fields"`
	AllowErrors   bool   `name:"allow-errors" help:"Continue on item-level validation errors"`
	DryRun        bool   `name:"dry-run" help:"Reserved for parity; currently reports planned selection only"`
}

// Run executes entries copy.
func (c *ChannelEntriesCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "copy channel entries"); err != nil {
			return err
		}
	}
	fromRef, err := parseChannelRefForCommand(ctx, c.From, c.FromHost)
	if err != nil {
		return err
	}
	toRef, err := parseChannelRefForCommand(ctx, c.To, c.ToHost)
	if err != nil {
		return err
	}
	fromClient, err := GetAPIClientWithBaseURL(ctx, fromRef.BaseURL, fromRef.Site)
	if err != nil {
		return err
	}
	toClient, err := GetAPIClientWithBaseURL(ctx, toRef.BaseURL, toRef.Site)
	if err != nil {
		return err
	}
	ctx, tl := copyWithTimeline(ctx, "Channel Entries", fromRef.Site, toRef.Site, c.DryRun)
	if tl != nil {
		defer tl.Close()
	}
	result, err := migrate.CopyChannelEntries(ctx, fromClient, toClient, fromRef, toRef, migrate.RecordCopyOptions{
		AllowErrors:   c.AllowErrors,
		CopyCustomers: c.CopyCustomers,
		DryRun:        c.DryRun,
		Only:          splitCSV(c.Only),
		PerPage:       c.PerPage,
		Query:         c.Query,
		Recursive:     c.Recursive,
		Upsert:        c.Upsert,
		Where:         c.Where,
	})
	if err != nil {
		return finishCopyTimelineError(tl, err)
	}
	finishCopyTimeline(tl, "Channel Entries", fmt.Sprintf("%d entries", len(result.Items)))
	if tl != nil {
		return nil
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	for _, item := range result.Items {
		if mode.Plain {
			if _, err := output.Fprintf(ctx, "%s\t%s\t%s\t%s\n", item.Action, item.Resource, item.Identifier, item.TargetID); err != nil {
				return err
			}
			continue
		}
		if _, err := output.Fprintf(ctx, "%s %s %s\n", item.Action, item.Resource, item.Identifier); err != nil {
			return err
		}
	}
	for _, warning := range result.Warnings {
		if _, err := output.Fprintf(ctx, "warning: %s\n", warning); err != nil {
			return err
		}
	}
	return nil
}
