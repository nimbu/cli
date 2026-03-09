package cmd

import (
	"context"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// BlogsCopyCmd copies blogs and their posts between sites.
type BlogsCopyCmd struct {
	Handle   string `arg:"" optional:"" help:"Blog handle to copy" default:"*"`
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
	result, err := migrate.CopyBlogs(ctx, fromClient, toClient, fromRef, toRef, c.Handle, nil)
	if err != nil {
		return err
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	for _, item := range result.Items {
		label := item.Blog
		if item.Slug != "" {
			label = item.Blog + "/" + item.Slug
		}
		if mode.Plain {
			if err := printLine(ctx, "%s\t%s\t%s\n", item.Action, item.Kind, label); err != nil {
				return err
			}
			continue
		}
		if err := printLine(ctx, "%s %s %s\n", item.Action, item.Kind, label); err != nil {
			return err
		}
	}
	return nil
}
