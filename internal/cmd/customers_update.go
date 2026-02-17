package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CustomersUpdateCmd updates a customer.
type CustomersUpdateCmd struct {
	Customer    string   `arg:"" help:"Customer ID or email"`
	File        string   `help:"Read customer JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. first_name=Ana, phone=+32...)"`
}

// Run executes the update command.
func (c *CustomersUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update customer"); err != nil {
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

	var cust api.Customer
	path := "/customers/" + url.PathEscape(c.Customer)
	if err := client.Put(ctx, path, body, &cust); err != nil {
		return fmt.Errorf("update customer: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, cust)
	}

	if mode.Plain {
		return output.Plain(ctx, cust.ID, cust.Email)
	}

	fmt.Printf("Updated customer: %s (%s)\n", cust.Email, cust.ID)
	return nil
}
