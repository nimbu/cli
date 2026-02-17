package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CustomersListCmd lists customers.
type CustomersListCmd struct{}

// Run executes the list command.
func (c *CustomersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	customers, err := api.List[api.Customer](ctx, client, "/customers")
	if err != nil {
		return fmt.Errorf("list customers: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, customers)
	}

	if mode.Plain {
		for _, cust := range customers {
			if err := output.Plain(ctx, cust.ID, cust.Email, cust.FirstName, cust.LastName); err != nil {
				return err
			}
		}
		return nil
	}

	// Human-readable table
	fields := []string{"id", "email", "first_name", "last_name"}
	headers := []string{"ID", "EMAIL", "FIRST NAME", "LAST NAME"}
	return output.WriteTable(ctx, customers, fields, headers)
}
