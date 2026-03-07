package cmd

import (
	"context"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// SitesCopyCmd copies major site resources between sites.
type SitesCopyCmd struct {
	From          string `help:"Source site" required:"" name:"from"`
	To            string `help:"Target site" required:"" name:"to"`
	FromHost      string `help:"Source API base URL or host" name:"from-host"`
	ToHost        string `help:"Target API base URL or host" name:"to-host"`
	EntryChannels string `help:"Comma-separated channels whose entries should also be copied" name:"entry-channels"`
	Only          string `help:"Comma-separated channel allowlist when using --recursive"`
	Recursive     bool   `help:"Recursively copy dependent channel entries"`
	Upsert        string `help:"Comma-separated upsert fields for entry-copy stage"`
	CopyCustomers bool   `name:"copy-customers" help:"Copy related customers when copying channel entries"`
	AllowErrors   bool   `name:"allow-errors" help:"Continue on item-level validation errors during record copy"`
}

// Run executes sites copy.
func (c *SitesCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "copy site"); err != nil {
		return err
	}
	fromRef, err := parseSiteRefForCommand(ctx, c.From, c.FromHost)
	if err != nil {
		return err
	}
	toRef, err := parseSiteRefForCommand(ctx, c.To, c.ToHost)
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
	result, err := migrate.CopySite(ctx, fromClient, toClient, fromRef, toRef, migrate.SiteCopyOptions{
		AllowErrors:   c.AllowErrors,
		CopyCustomers: c.CopyCustomers,
		Force:         flags != nil && flags.Force,
		Include:       splitCSV(c.EntryChannels),
		Only:          splitCSV(c.Only),
		Recursive:     c.Recursive,
		Upsert:        c.Upsert,
	})
	if err != nil {
		return err
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		if err := printLine(ctx, "channels\t%d\n", len(result.Channels.Items)); err != nil {
			return err
		}
		if err := printLine(ctx, "channel_entries\t%d\n", len(result.ChannelEntries)); err != nil {
			return err
		}
		if err := printLine(ctx, "pages\t%d\n", len(result.Pages.Items)); err != nil {
			return err
		}
		if err := printLine(ctx, "menus\t%d\n", len(result.Menus.Items)); err != nil {
			return err
		}
		return printLine(ctx, "translations\t%d\n", len(result.Translations.Items))
	}
	if err := printLine(ctx, "channels: %d\n", len(result.Channels.Items)); err != nil {
		return err
	}
	if err := printLine(ctx, "channel entry stages: %d\n", len(result.ChannelEntries)); err != nil {
		return err
	}
	if err := printLine(ctx, "pages: %d\n", len(result.Pages.Items)); err != nil {
		return err
	}
	if err := printLine(ctx, "menus: %d\n", len(result.Menus.Items)); err != nil {
		return err
	}
	return printLine(ctx, "translations: %d\n", len(result.Translations.Items))
}
