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
	Blog   string `required:"" help:"Blog ID or handle"`
	Post   string `required:"" help:"Post ID or slug"`
	Locale string `help:"Content locale for localized post fields"`
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
	var document api.Document[api.BlogPost]
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Get(ctx, path, &document, opts...); err != nil {
		return fmt.Errorf("get article: %w", err)
	}
	post, err := localizedDocumentMap(document, c.Locale)
	if err != nil {
		return fmt.Errorf("project article locale: %w", err)
	}

	return output.Detail(ctx, document, []any{post["id"], post["slug"], post["title"], post["status"]}, []output.Field{
		output.FAlways("ID", post["id"]),
		output.FAlways("Slug", post["slug"]),
		output.FAlways("Title", post["title"]),
		output.FAlways("Status", post["status"]),
		output.F("Author", post["author"]),
		output.F("Body", post["text_content"]),
	})
}
