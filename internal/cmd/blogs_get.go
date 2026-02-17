package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogsGetCmd gets blog details.
type BlogsGetCmd struct {
	Blog string `arg:"" help:"Blog ID or handle"`
}

// Run executes the get command.
func (c *BlogsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var blog api.Blog
	path := "/blogs/" + url.PathEscape(c.Blog)
	if err := client.Get(ctx, path, &blog); err != nil {
		return fmt.Errorf("get blog: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, blog)
	}

	if mode.Plain {
		return output.Plain(ctx, blog.ID, blog.Handle, blog.Name)
	}

	fmt.Printf("ID:     %s\n", blog.ID)
	fmt.Printf("Handle: %s\n", blog.Handle)
	fmt.Printf("Name:   %s\n", blog.Name)

	return nil
}
