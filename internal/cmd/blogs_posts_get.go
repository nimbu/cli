package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogPostsGetCmd gets a blog article.
type BlogPostsGetCmd struct {
	Blog string `arg:"" help:"Blog ID or handle"`
	Post string `arg:"" help:"Post ID or slug"`
}

// Run executes the get command.
func (c *BlogPostsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/blogs/" + url.PathEscape(c.Blog) + "/articles/" + url.PathEscape(c.Post)
	var post api.BlogPost
	if err := client.Get(ctx, path, &post); err != nil {
		return fmt.Errorf("get article: %w", err)
	}

	return output.Detail(ctx, post, []any{post.ID, post.Slug, post.Title, post.Status}, []output.Field{
		output.FAlways("ID", post.ID),
		output.FAlways("Slug", post.Slug),
		output.FAlways("Title", post.Title),
		output.FAlways("Status", post.Status),
		output.F("Author", post.Author),
		output.F("Body", post.TextContent),
	})
}
