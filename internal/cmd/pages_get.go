package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PagesGetCmd gets page details.
type PagesGetCmd struct {
	Page string `arg:"" help:"Page ID or slug"`
}

// Run executes the get command.
func (c *PagesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var opts []api.RequestOption
	if flags.Locale != "" {
		opts = append(opts, api.WithLocale(flags.Locale))
	}

	var page api.Page
	path := "/pages/" + url.PathEscape(c.Page)
	if err := client.Get(ctx, path, &page, opts...); err != nil {
		return fmt.Errorf("get page: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, page)
	}

	if mode.Plain {
		return output.Plain(ctx, page.ID, page.Slug, page.Title, page.Published)
	}

	fmt.Printf("ID:        %s\n", page.ID)
	fmt.Printf("Slug:      %s\n", page.Slug)
	fmt.Printf("Title:     %s\n", page.Title)
	if page.Template != "" {
		fmt.Printf("Template:  %s\n", page.Template)
	}
	fmt.Printf("Published: %v\n", page.Published)
	if page.Locale != "" {
		fmt.Printf("Locale:    %s\n", page.Locale)
	}

	return nil
}
