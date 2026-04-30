package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// ProductsDeleteCmd deletes a product.
type ProductsDeleteCmd struct {
	Product string `required:"" help:"Product ID or slug"`
}

// Run executes the delete command.
func (c *ProductsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete product"); err != nil {
		return err
	}

	if err := requireForce(flags, "product "+c.Product); err != nil {
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

	path := "/products/" + url.PathEscape(c.Product)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete product: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("product deleted"), []any{c.Product, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted product: %s\n", c.Product)
		return err
	})
}
