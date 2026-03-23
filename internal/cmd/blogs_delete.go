package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// BlogsDeleteCmd deletes a blog.
type BlogsDeleteCmd struct {
	Blog string `arg:"" help:"Blog ID or handle"`
}

// Run executes the delete command.
func (c *BlogsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete blog"); err != nil {
		return err
	}
	if err := requireForce(flags, "blog "+c.Blog); err != nil {
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

	path := "/blogs/" + url.PathEscape(c.Blog)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete blog: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload(fmt.Sprintf("deleted %s", c.Blog)), []any{c.Blog, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted blog: %s\n", c.Blog)
		return err
	})
}
