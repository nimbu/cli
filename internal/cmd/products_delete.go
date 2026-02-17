package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/output"
)

// ProductsDeleteCmd deletes a product.
type ProductsDeleteCmd struct {
	Product string `arg:"" help:"Product ID or slug"`
}

// Run executes the delete command.
func (c *ProductsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("cannot delete product in readonly mode")
	}

	if !flags.Force {
		return fmt.Errorf("use --force to confirm deletion of product %s", c.Product)
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	if err := client.Delete(ctx, "/products/"+c.Product, nil); err != nil {
		return fmt.Errorf("delete product: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("product deleted"))
	}

	if mode.Plain {
		return output.Plain(ctx, c.Product, "deleted")
	}

	fmt.Printf("Deleted product: %s\n", c.Product)
	return nil
}
