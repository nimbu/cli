package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogPostsUpdateCmd updates a blog article.
type BlogPostsUpdateCmd struct {
	Blog        string   `required:"" help:"Blog ID or handle"`
	Post        string   `required:"" help:"Post ID or slug"`
	Locale      string   `help:"Content locale for localized post fields"`
	File        string   `help:"Read post JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. title=Hello, status=published)"`
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
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Put(ctx, path, body, &result, opts...); err != nil {
		return fmt.Errorf("update article: %w", err)
	}

	projected, err := output.ProjectLocale(result, c.Locale)
	if err != nil {
		return fmt.Errorf("project article locale: %w", err)
	}
	display := projected.(map[string]any)
	return output.Print(ctx, result, []any{display["id"], display["slug"], display["title"]}, func() error {
		_, err := output.Fprintf(ctx, "Updated article in blog %s\n", c.Blog)
		return err
	})
}
