package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// CustomersDeleteCmd deletes a customer.
type CustomersDeleteCmd struct {
	Customer string `required:"" help:"Customer ID or email"`
}

// Run executes the delete command.
func (c *CustomersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete customer"); err != nil {
		return err
	}

	if err := requireForce(flags, "customer "+c.Customer); err != nil {
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

	path := "/customers/" + url.PathEscape(c.Customer)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete customer: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("customer deleted"), []any{c.Customer, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted customer: %s\n", c.Customer)
		return err
	})
}
