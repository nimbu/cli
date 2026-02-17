package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/output"
)

// CustomersDeleteCmd deletes a customer.
type CustomersDeleteCmd struct {
	Customer string `arg:"" help:"Customer ID or email"`
}

// Run executes the delete command.
func (c *CustomersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("cannot delete customer in readonly mode")
	}

	if !flags.Force {
		return fmt.Errorf("use --force to confirm deletion of customer %s", c.Customer)
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	if err := client.Delete(ctx, "/customers/"+c.Customer, nil); err != nil {
		return fmt.Errorf("delete customer: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("customer deleted"))
	}

	if mode.Plain {
		return output.Plain(ctx, c.Customer, "deleted")
	}

	fmt.Printf("Deleted customer: %s\n", c.Customer)
	return nil
}
