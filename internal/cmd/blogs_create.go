package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogsCreateCmd creates a blog.
type BlogsCreateCmd struct {
	Locale      string   `help:"Content locale for localized blog fields"`
	File        string   `help:"Read blog JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Blog, slug=news)"`
}

// Run executes the create command.
func (c *BlogsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create blog"); err != nil {
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

	var document api.Document[api.Blog]
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Post(ctx, "/blogs", body, &document, opts...); err != nil {
		return fmt.Errorf("create blog: %w", err)
	}
	blog := document.Value
	display, err := localizedDocumentMap(document, c.Locale)
	if err != nil {
		return fmt.Errorf("project blog locale: %w", err)
	}
	applyBlogDisplayHandle(display)

	return output.Print(ctx, document, []any{display["id"], display["handle"], display["name"]}, func() error {
		_, err := output.Fprintf(ctx, "Created blog: %s\n", blog.ID)
		return err
	})
}
