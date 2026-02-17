package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogsUpdateCmd updates a blog.
type BlogsUpdateCmd struct {
	Blog        string   `arg:"" help:"Blog ID or handle"`
	File        string   `help:"Read blog JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Blog, handle=news)"`
}

// Run executes the update command.
func (c *BlogsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update blog"); err != nil {
		return err
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	body, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		return err
	}

	var blog api.Blog
	path := "/blogs/" + url.PathEscape(c.Blog)
	if err := client.Put(ctx, path, body, &blog); err != nil {
		return fmt.Errorf("update blog: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, blog)
	}

	if mode.Plain {
		return output.Plain(ctx, blog.ID, blog.Handle, blog.Name)
	}

	fmt.Printf("Updated blog: %s\n", blog.ID)
	return nil
}
