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
	DryRun        bool   `name:"dry-run" help:"Show what would be copied without writing to target site"`
}

// Run executes sites copy.
func (c *SitesCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "copy site"); err != nil {
			return err
		}
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

	mode := output.FromContext(ctx)

	// Wire timeline for human mode
	var tl *output.CopyTimeline
	if !mode.JSON && !mode.Plain {
		tl = output.NewCopyTimeline(ctx, c.DryRun)
		ctx = migrate.WithCopyObserver(ctx, tl)
		ctx = output.WithProgress(ctx, output.NewDisabledProgress())
		tl.Header(fromRef.Site, toRef.Site)
	}

	result, err := migrate.CopySite(ctx, fromClient, toClient, fromRef, toRef, migrate.SiteCopyOptions{
		AllowErrors:   c.AllowErrors,
		CopyCustomers: c.CopyCustomers,
		DryRun:        c.DryRun,
		Force:         flags != nil && flags.Force,
		Include:       splitCSV(c.EntryChannels),
		Only:          splitCSV(c.Only),
		Recursive:     c.Recursive,
		Upsert:        c.Upsert,
	})

	if tl != nil {
		if err == nil {
			tl.Footer()
		} else {
			tl.ErrorFooter(err.Error())
			err = &displayedError{err: err}
		}
		tl.Close()
	}

	if err != nil {
		return err
	}

	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		prefix := ""
		if result.DryRun {
			prefix = "[dry-run] "
		}
		if _, err := output.Fprintf(ctx, "%suploads\t%d\n", prefix, len(result.Uploads.Items)); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "%schannels\t%d\n", prefix, len(result.Channels.Items)); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "%schannel_entries\t%d\n", prefix, len(result.ChannelEntries)); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "%sroles\t%d\n", prefix, len(result.Roles.Items)); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "%sproducts\t%d\n", prefix, len(result.Products.Items)); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "%scollections\t%d\n", prefix, len(result.Collections.Items)); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "%spages\t%d\n", prefix, len(result.Pages.Items)); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "%smenus\t%d\n", prefix, len(result.Menus.Items)); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "%sblogs\t%d\n", prefix, len(result.Blogs.Items)); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "%snotifications\t%d\n", prefix, len(result.Notifications.Items)); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "%sredirects\t%d\n", prefix, len(result.Redirects.Items)); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "%stranslations\t%d\n", prefix, len(result.Translations.Items)); err != nil {
			return err
		}
		_, err := output.Fprintf(ctx, "%swarnings\t%d\n", prefix, len(result.Warnings))
		return err
	}

	// Human mode: timeline already rendered during execution
	return nil
}
