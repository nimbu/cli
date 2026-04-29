package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// MenusCopyCmd copies menus between sites.
type MenusCopyCmd struct {
	Slug     string `help:"Menu slug to copy" default:"*" name:"only"`
	From     string `help:"Source site" required:"" name:"from"`
	To       string `help:"Target site" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
}

// Run executes menus copy.
func (c *MenusCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "copy menus"); err != nil {
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
	overwrite := flags != nil && flags.Force
	if !overwrite && c.Slug != "" && c.Slug != "*" {
		var existing api.MenuDocument
		path := "/menus/" + url.PathEscape(c.Slug)
		if err := toClient.Get(ctx, path, &existing); err == nil {
			ok, err := confirmPrompt(flags, fmt.Sprintf("overwrite existing menu %s", c.Slug))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("aborted")
			}
			overwrite = true
		}
	}
	ctx, tl := copyWithTimeline(ctx, "Menus", fromRef.Site, toRef.Site, false)
	if tl != nil {
		defer tl.Close()
	}
	result, err := migrate.CopyMenus(ctx, fromClient, toClient, fromRef, toRef, c.Slug, overwrite, nil, false)
	if err != nil {
		return finishCopyTimelineError(tl, err)
	}
	finishCopyTimeline(tl, "Menus", fmt.Sprintf("%d synced", len(result.Items)))
	if tl != nil {
		return nil
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	for _, item := range result.Items {
		if mode.Plain {
			if _, err := output.Fprintf(ctx, "%s\t%s\n", item.Action, item.Slug); err != nil {
				return err
			}
			continue
		}
		if _, err := output.Fprintf(ctx, "%s %s\n", item.Action, item.Slug); err != nil {
			return err
		}
	}
	return nil
}
