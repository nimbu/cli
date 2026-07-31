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
	Blog   string `required:"" help:"Blog ID or handle"`
	Locale string `help:"Content locale for localized blog fields"`
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

	var document api.Document[api.Blog]
	path := "/blogs/" + url.PathEscape(c.Blog)
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Get(ctx, path, &document, opts...); err != nil {
		return fmt.Errorf("get blog: %w", err)
	}
	blog, err := localizedDocumentMap(document, c.Locale)
	if err != nil {
		return fmt.Errorf("project blog locale: %w", err)
	}
	applyBlogDisplayHandle(blog)

	return output.Detail(ctx, document, []any{blog["id"], blog["handle"], blog["name"]}, []output.Field{
		output.FAlways("ID", blog["id"]),
		output.FAlways("Handle", blog["handle"]),
		output.FAlways("Name", blog["name"]),
	})
}
