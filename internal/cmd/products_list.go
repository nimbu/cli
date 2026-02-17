package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ProductsListCmd lists products.
type ProductsListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

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

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list products: %w", err)
	}

	var products []api.Product

	if c.All {
		products, err = api.List[api.Product](ctx, client, "/products", opts...)
		if err != nil {
			return fmt.Errorf("list products: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Product](ctx, client, "/products", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list products: %w", err)
		}
		products = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, products)
	}

	plainFields := []string{"id", "slug", "name", "sku", "price"}
	tableFields := []string{"id", "slug", "name", "sku", "price", "published"}
	tableHeaders := []string{"ID", "SLUG", "NAME", "SKU", "PRICE", "PUBLISHED"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, products, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, products, fields, headers)
}
