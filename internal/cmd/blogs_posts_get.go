package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogPostsGetCmd gets a blog post.
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

	path := "/blogs/" + c.Blog + "/posts/" + c.Post
	var post api.BlogPost
	if err := client.Get(ctx, path, &post); err != nil {
		return fmt.Errorf("get post: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, post)
	}

	if mode.Plain {
		return output.Plain(ctx, post.ID, post.Slug, post.Title, post.Published)
	}

	fmt.Printf("ID:        %s\n", post.ID)
	fmt.Printf("Slug:      %s\n", post.Slug)
	fmt.Printf("Title:     %s\n", post.Title)
	fmt.Printf("Published: %v\n", post.Published)
	if post.Author != "" {
		fmt.Printf("Author:    %s\n", post.Author)
	}
	if post.Body != "" {
		fmt.Printf("Body:      %s\n", post.Body)
	}

	return nil
}
