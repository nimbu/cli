package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PagesCreateCmd creates a page.
type PagesCreateCmd struct {
	Locale      string   `help:"Content locale for localized page fields"`
	File        string   `help:"Read page JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. slug=about, title=About)"`
}

// Run executes the create command.
func (c *PagesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create page"); err != nil {
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
	if err := api.ExpandPageAttachmentPathsWithOptions(api.PageDocument(body), pageAttachmentFileRefOptions(ctx, client, api.PageAttachmentExpansionOptions{})); err != nil {
		return err
	}

	var document api.Document[api.Page]
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Post(ctx, "/pages", body, &document, opts...); err != nil {
		return withPageThemeHint(fmt.Errorf("create page: %w", err), stringAny(body["template"]))
	}
	page := document.Value

	return output.Print(ctx, document, []any{page.ID}, func() error {
		_, err := output.Fprintf(ctx, "Created page %s\n", page.ID)
		return err
	})
}
