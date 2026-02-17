package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// BlogPostsCreateCmd creates a blog article.
type BlogPostsCreateCmd struct {
	Blog        string   `arg:"" help:"Blog ID or handle"`
	File        string   `help:"Read post JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. title=Hello, published:=true)"`
}

// Run executes the create command.
func (c *BlogPostsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create post"); err != nil {
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
	path := "/blogs/" + url.PathEscape(c.Blog) + "/articles"
	if err := client.Post(ctx, path, body, &result); err != nil {
		return fmt.Errorf("create article: %w", err)
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

	fmt.Printf("Created article in blog %s\n", c.Blog)
	return nil
}
