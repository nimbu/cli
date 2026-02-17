package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CustomersGetCmd gets a customer by ID or email.
type CustomersGetCmd struct {
	Customer string `arg:"" help:"Customer ID or email"`
}

// Run executes the get command.
func (c *CustomersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var cust api.Customer
	if err := client.Get(ctx, "/customers/"+c.Customer, &cust); err != nil {
		return fmt.Errorf("get customer: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, cust)
	}

	if mode.Plain {
		return output.Plain(ctx, cust.ID, cust.Email, cust.FirstName, cust.LastName, cust.Phone)
	}

	fmt.Printf("ID:         %s\n", cust.ID)
	fmt.Printf("Email:      %s\n", cust.Email)
	if cust.FirstName != "" {
		fmt.Printf("First Name: %s\n", cust.FirstName)
	}
	if cust.LastName != "" {
		fmt.Printf("Last Name:  %s\n", cust.LastName)
	}
	if cust.Phone != "" {
		fmt.Printf("Phone:      %s\n", cust.Phone)
	}
	if !cust.CreatedAt.IsZero() {
		fmt.Printf("Created:    %s\n", cust.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if !cust.UpdatedAt.IsZero() {
		fmt.Printf("Updated:    %s\n", cust.UpdatedAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}
