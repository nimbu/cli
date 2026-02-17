package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ProductsListCmd lists products.
type ProductsListCmd struct{}

// Run executes the list command.
func (c *ProductsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	products, err := api.List[api.Product](ctx, client, "/products")
	if err != nil {
		return fmt.Errorf("list products: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, products)
	}

	if mode.Plain {
		for _, p := range products {
			if err := output.Plain(ctx, p.ID, p.Slug, p.Name, p.SKU, p.Price); err != nil {
				return err
			}
		}
		return nil
	}

	// Human-readable table
	fields := []string{"id", "slug", "name", "sku", "price", "published"}
	headers := []string{"ID", "SLUG", "NAME", "SKU", "PRICE", "PUBLISHED"}
	return output.WriteTable(ctx, products, fields, headers)
}
