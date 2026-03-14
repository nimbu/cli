package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsCopyCmd copies one or all channels between sites.
type ChannelsCopyCmd struct {
	All      bool   `help:"Copy all channels from source to target"`
	From     string `help:"Source site/channel or site when using --all" required:"" name:"from"`
	To       string `help:"Target site/channel or site when using --all" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
}

// Run executes channel copy.
func (c *ChannelsCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "copy channels"); err != nil {
		return err
	}

	if c.All {
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
		ctx, tl := copyWithTimeline(ctx, "Channels", fromRef.Site, toRef.Site, false)
		if tl != nil {
			defer tl.Close()
		}
		result, err := migrate.CopyAllChannels(ctx, fromClient, toClient, fromRef, toRef, false)
		if err != nil {
			return finishCopyTimelineError(tl, err)
		}
		finishCopyTimeline(tl, "Channels", fmt.Sprintf("%d synced", len(result.Items)))
		if tl != nil {
			return nil
		}
		return writeChannelCopyResult(ctx, result)
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
	ctx, tl := copyWithTimeline(ctx, "Channels", fromRef.Site, toRef.Site, false)
	if tl != nil {
		defer tl.Close()
	}
	result, err := migrate.CopyChannel(ctx, fromClient, toClient, fromRef, toRef)
	if err != nil {
		return finishCopyTimelineError(tl, err)
	}
	finishCopyTimeline(tl, "Channels", fmt.Sprintf("%d synced", len(result.Items)))
	if tl != nil {
		return nil
	}
	return writeChannelCopyResult(ctx, result)
}

func writeChannelCopyResult(ctx context.Context, result migrate.ChannelCopyResult) error {
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, item := range result.Items {
			if err := printLine(ctx, "%s\t%s\t%s\n", item.Action, item.Source, item.Target); err != nil {
				return err
			}
		}
		return nil
	}
	for _, item := range result.Items {
		if err := printLine(ctx, "%s %s -> %s\n", item.Action, item.Source, item.Target); err != nil {
			return err
		}
	}
	if len(result.Placeholders) > 0 {
		if err := printLine(ctx, "placeholders: %s\n", fmt.Sprintf("%v", result.Placeholders)); err != nil {
			return err
		}
	}
	return nil
}
