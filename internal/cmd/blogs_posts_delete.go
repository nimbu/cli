package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// BlogPostsDeleteCmd deletes a blog article.
type BlogPostsDeleteCmd struct {
	Blog string `arg:"" help:"Blog ID or handle"`
	Post string `arg:"" help:"Post ID or slug"`
}

// Run executes the delete command.
func (c *BlogPostsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete post"); err != nil {
		return err
	}
	if err := requireForce(flags, "post "+c.Post); err != nil {
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

	path := "/blogs/" + url.PathEscape(c.Blog) + "/articles/" + url.PathEscape(c.Post)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete article: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload(fmt.Sprintf("deleted %s", c.Post)))
	}

	if mode.Plain {
		return output.Plain(ctx, c.Post, "deleted")
	}

	fmt.Printf("Deleted post %s\n", c.Post)
	return nil
}
