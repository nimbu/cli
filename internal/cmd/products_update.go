package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ProductsUpdateCmd updates a product.
type ProductsUpdateCmd struct {
	Product     string   `required:"" help:"Product ID or slug"`
	Locale      string   `help:"Content locale for localized product fields"`
	File        string   `help:"Read product JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Wine, price:=19.9)"`
}

// Run executes the update command.
func (c *ProductsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update product"); err != nil {
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

	var document api.Document[api.Product]
	path := "/products/" + url.PathEscape(c.Product)
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Put(ctx, path, body, &document, opts...); err != nil {
		return fmt.Errorf("update product: %w", err)
	}
	p := document.Value
	display, err := localizedDocumentMap(document, c.Locale)
	if err != nil {
		return fmt.Errorf("project product locale: %w", err)
	}

	return output.Print(ctx, document, []any{display["id"], display["slug"], display["name"]}, func() error {
		_, err := output.Fprintf(ctx, "Updated product: %v (%s)\n", display["name"], p.ID)
		return err
	})
}
