package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// BlogPostsUpdateCmd updates a blog article.
type BlogPostsUpdateCmd struct {
	Blog        string   `arg:"" help:"Blog ID or handle"`
	Post        string   `arg:"" help:"Post ID or slug"`
	File        string   `help:"Read post JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. title=Hello, published:=true)"`
}

// Run executes the update command.
func (c *BlogPostsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update post"); err != nil {
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

	var result map[string]any
	path := "/blogs/" + url.PathEscape(c.Blog) + "/articles/" + url.PathEscape(c.Post)
	if err := client.Put(ctx, path, body, &result); err != nil {
		return fmt.Errorf("update article: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}

	if mode.Plain {
		id, _ := result["id"].(string)
		slug, _ := result["slug"].(string)
		title, _ := result["title"].(string)
		return output.Plain(ctx, id, slug, title)
	}

	fmt.Printf("Updated article in blog %s\n", c.Blog)
	return nil
}
