package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogPostsCountCmd gets count of articles for a blog.
type BlogPostsCountCmd struct {
	Blog string `arg:"" help:"Blog ID or handle"`
}

// Run executes the count command.
func (c *BlogPostsCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/blogs/" + url.PathEscape(c.Blog) + "/articles/count"
	count, err := api.Count(ctx, client, path)
	if err != nil {
		return fmt.Errorf("count articles: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.CountPayload(count))
	}

	if mode.Plain {
		return output.Plain(ctx, count)
	}

	fmt.Printf("Articles: %d\n", count)
	return nil
}
