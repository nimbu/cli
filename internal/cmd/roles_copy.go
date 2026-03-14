package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// RolesCopyCmd copies customer roles between sites.
type RolesCopyCmd struct {
	From     string `help:"Source site" required:"" name:"from"`
	To       string `help:"Target site" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
}

// Run executes roles copy.
func (c *RolesCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "copy roles"); err != nil {
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
	ctx, tl := copyWithTimeline(ctx, "Roles", fromRef.Site, toRef.Site, false)
	if tl != nil {
		defer tl.Close()
	}
	result, err := migrate.CopyRoles(ctx, fromClient, toClient, fromRef, toRef, false)
	if err != nil {
		return finishCopyTimelineError(tl, err)
	}
	finishCopyTimeline(tl, "Roles", fmt.Sprintf("%d synced", len(result.Items)))
	if tl != nil {
		return nil
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	for _, item := range result.Items {
		if mode.Plain {
			if err := printLine(ctx, "%s\t%s\n", item.Action, item.Name); err != nil {
				return err
			}
			continue
		}
		if err := printLine(ctx, "%s %s\n", item.Action, item.Name); err != nil {
			return err
		}
	}
	return nil
}
