package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ProductsCreateCmd creates a product.
type ProductsCreateCmd struct {
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

	var p api.Product
	if err := client.Post(ctx, "/products", body, &p); err != nil {
		return fmt.Errorf("create product: %w", err)
	}

	return output.Print(ctx, p, []any{p.ID, p.Slug, p.Name}, func() error {
		_, err := output.Fprintf(ctx, "Created product: %s (%s)\n", p.Name, p.ID)
		return err
	})
}
