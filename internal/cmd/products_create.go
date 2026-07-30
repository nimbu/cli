package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ProductsCreateCmd creates a product.
type ProductsCreateCmd struct {
	Locale      string   `help:"Content locale for localized product fields"`
	File        string   `help:"Read product JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Wine, price:=19.9)"`
}

// Run executes the create command.
func (c *ProductsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create product"); err != nil {
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
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Post(ctx, "/products", body, &document, opts...); err != nil {
		return fmt.Errorf("create product: %w", err)
	}
	p := document.Value

	return output.Print(ctx, document, []any{p.ID, p.Slug, p.Name}, func() error {
		_, err := output.Fprintf(ctx, "Created product: %s (%s)\n", p.Name, p.ID)
		return err
	})
}
