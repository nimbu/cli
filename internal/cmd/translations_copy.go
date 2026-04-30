package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// TranslationsCopyCmd copies translations between sites.
type TranslationsCopyCmd struct {
	Query    string `help:"Translation key, prefix*, or *" default:"*" name:"only"`
	From     string `help:"Source site" required:"" name:"from"`
	To       string `help:"Target site" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
	Since    string `help:"Only copy translations updated since RFC3339 or relative duration like 1d"`
	DryRun   bool   `name:"dry-run" help:"Show what would be copied without writing target state"`
}

// Run executes translations copy.
func (c *TranslationsCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "copy translations"); err != nil {
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
	since, err := parseSinceFlag(c.Since)
	if err != nil {
		return err
	}
	ctx, tl := copyWithTimeline(ctx, "Translations", fromRef.Site, toRef.Site, c.DryRun)
	if tl != nil {
		defer tl.Close()
	}
	result, err := migrate.CopyTranslations(ctx, fromClient, toClient, fromRef, toRef, migrate.TranslationCopyOptions{
		Query:  c.Query,
		Since:  since,
		DryRun: c.DryRun,
	})
	if err != nil {
		return finishCopyTimelineError(tl, err)
	}
	finishCopyTimeline(tl, "Translations", fmt.Sprintf("%d synced", len(result.Items)))
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, item := range result.Items {
			if _, err := output.Fprintf(ctx, "%s\t%s\n", item.Action, item.Key); err != nil {
				return err
			}
		}
		return nil
	}
	if tl != nil {
		return nil
	}
	for _, item := range result.Items {
		if _, err := output.Fprintf(ctx, "%s %s\n", item.Action, item.Key); err != nil {
			return err
		}
	}
	return nil
}
