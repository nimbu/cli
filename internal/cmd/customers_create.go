package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CustomersCreateCmd creates a customer.
type CustomersCreateCmd struct {
	File        string   `help:"Read customer JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. email=a@b.com, first_name=Ana)"`
}

// Run executes the create command.
func (c *CustomersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create customer"); err != nil {
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
	if err := client.Post(ctx, "/customers", body, &cust); err != nil {
		return fmt.Errorf("create customer: %w", err)
	}

	return output.Print(ctx, cust, []any{cust.ID, cust.Email}, func() error {
		_, err := output.Fprintf(ctx, "Created customer: %s (%s)\n", cust.Email, cust.ID)
		return err
	})
}
