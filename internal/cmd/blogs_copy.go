package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// BlogsCopyCmd copies blogs and their posts between sites.
type BlogsCopyCmd struct {
	Handle   string `help:"Blog handle to copy" default:"*" name:"only"`
	From     string `help:"Source site" required:"" name:"from"`
	To       string `help:"Target site" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
}

// Run executes blogs copy.
func (c *BlogsCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "copy blogs"); err != nil {
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
	ctx, tl := copyWithTimeline(ctx, "Blogs", fromRef.Site, toRef.Site, false)
	if tl != nil {
		defer tl.Close()
	}
	result, err := migrate.CopyBlogs(ctx, fromClient, toClient, fromRef, toRef, c.Handle, nil, false)
	if err != nil {
		return finishCopyTimelineError(tl, err)
	}
	finishCopyTimeline(tl, "Blogs", fmt.Sprintf("%d synced", len(result.Items)))
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, item := range result.Items {
			label := item.Blog
			if item.Slug != "" {
				label = item.Blog + "/" + item.Slug
			}
			if _, err := output.Fprintf(ctx, "%s\t%s\t%s\n", item.Action, item.Kind, label); err != nil {
				return err
			}
		}
		return nil
	}
	if tl != nil {
		return nil
	}
	for _, item := range result.Items {
		label := item.Blog
		if item.Slug != "" {
			label = item.Blog + "/" + item.Slug
		}
		if _, err := output.Fprintf(ctx, "%s %s %s\n", item.Action, item.Kind, label); err != nil {
			return err
		}
	}
	return nil
}
