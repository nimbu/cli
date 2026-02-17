package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogPostsListCmd lists blog posts.
type BlogPostsListCmd struct {
	Blog    string `arg:"" help:"Blog ID or handle"`
	All     bool   `help:"Fetch all pages"`
	Page    int    `help:"Page number" default:"1"`
	PerPage int    `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *BlogPostsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/blogs/" + c.Blog + "/posts"
	var posts []api.BlogPost

	if c.All {
		posts, err = api.List[api.BlogPost](ctx, client, path)
		if err != nil {
			return fmt.Errorf("list posts: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.BlogPost](ctx, client, path, c.Page, c.PerPage)
		if err != nil {
			return fmt.Errorf("list posts: %w", err)
		}
		posts = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, posts)
	}

	if mode.Plain {
		for _, p := range posts {
			if err := output.Plain(ctx, p.ID, p.Slug, p.Title); err != nil {
				return err
			}
		}
		return nil
	}

	fields := []string{"id", "slug", "title", "published"}
	headers := []string{"ID", "SLUG", "TITLE", "PUBLISHED"}
	return output.WriteTable(ctx, posts, fields, headers)
}
