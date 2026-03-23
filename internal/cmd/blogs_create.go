package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogsCreateCmd creates a blog.
type BlogsCreateCmd struct {
	File        string   `help:"Read blog JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Blog, slug=news)"`
}

// Run executes the create command.
func (c *BlogsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create blog"); err != nil {
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
	if err := client.Post(ctx, "/blogs", body, &blog); err != nil {
		return fmt.Errorf("create blog: %w", err)
	}

	return output.Print(ctx, blog, []any{blog.ID, blog.Handle, blog.Name}, func() error {
		_, err := output.Fprintf(ctx, "Created blog: %s\n", blog.ID)
		return err
	})
}
