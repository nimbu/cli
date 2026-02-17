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
	Product     string   `arg:"" help:"Product ID or slug"`
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

	var p api.Product
	path := "/products/" + url.PathEscape(c.Product)
	if err := client.Put(ctx, path, body, &p); err != nil {
		return fmt.Errorf("update product: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, p)
	}

	if mode.Plain {
		return output.Plain(ctx, p.ID, p.Slug, p.Name)
	}

	fmt.Printf("Updated product: %s (%s)\n", p.Name, p.ID)
	return nil
}
