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
	if err := requireScopes(ctx, client, []string{"read_products"}, "Example: nimbu-cli auth scopes"); err != nil {
		return err
	}

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list products: %w", err)
	}

	var products []api.Product
	var meta listFooterMeta

	if c.All {
		products, err = api.List[api.Product](ctx, client, "/products", opts...)
		if err != nil {
			return fmt.Errorf("list products: %w", err)
		}
		meta = allListFooterMeta(len(products))
	} else {
		paged, err := api.ListPage[api.Product](ctx, client, "/products", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list products: %w", err)
		}
		products = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(products))
		meta.probeTotal(ctx, client, "/products/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, products)
	}

	plainFields := []string{"id", "slug", "name", "sku", "price", "status"}
	tableFields := []string{"id", "slug", "name", "sku", "price", "status"}
	tableHeaders := []string{"ID", "SLUG", "NAME", "SKU", "PRICE", "STATUS"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, products, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, products, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "products", meta)
}
