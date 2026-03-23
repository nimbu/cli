package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// PagesCopyCmd copies pages between sites.
type PagesCopyCmd struct {
	Fullpath string `arg:"" optional:"" help:"Page fullpath or prefix* to copy" default:"*"`
	From     string `help:"Source site" required:"" name:"from"`
	To       string `help:"Target site" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
}

// Run executes pages copy.
func (c *PagesCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "copy pages"); err != nil {
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
	ctx, tl := copyWithTimeline(ctx, "Pages", fromRef.Site, toRef.Site, false)
	if tl != nil {
		defer tl.Close()
	}
	result, err := migrate.CopyPages(ctx, fromClient, toClient, fromRef, toRef, c.Fullpath, nil, false)
	if err != nil {
		return finishCopyTimelineError(tl, err)
	}
	finishCopyTimeline(tl, "Pages", fmt.Sprintf("%d synced", len(result.Items)))
	for _, w := range result.Warnings {
		_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: %s\n", w)
	}
	if tl != nil {
		return nil
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	for _, item := range result.Items {
		if mode.Plain {
			if _, err := output.Fprintf(ctx, "%s\t%s\n", item.Action, item.Fullpath); err != nil {
				return err
			}
			continue
		}
		if _, err := output.Fprintf(ctx, "%s %s\n", item.Action, item.Fullpath); err != nil {
			return err
		}
	}
	return nil
}
