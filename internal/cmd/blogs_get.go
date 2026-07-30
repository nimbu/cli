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

	var blog api.Blog
	path := "/blogs/" + url.PathEscape(c.Blog)
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Get(ctx, path, &blog, opts...); err != nil {
		return fmt.Errorf("get blog: %w", err)
	}

	return output.Detail(ctx, blog, []any{blog.ID, blog.Handle, blog.Name}, []output.Field{
		output.FAlways("ID", blog.ID),
		output.FAlways("Handle", blog.Handle),
		output.FAlways("Name", blog.Name),
	})
}
