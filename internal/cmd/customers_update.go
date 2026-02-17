package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CustomersUpdateCmd updates a customer.
type CustomersUpdateCmd struct {
	Customer string `arg:"" help:"Customer ID or email"`
	File     string `help:"Read customer JSON from file (use - for stdin)" type:"existingfile"`
}

// Run executes the update command.
func (c *CustomersUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("cannot update customer in readonly mode")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	// Read input
	var input io.Reader
	if c.File == "-" || c.File == "" {
		input = os.Stdin
	} else {
		f, err := os.Open(c.File)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}
		defer func() { _ = f.Close() }()
		input = f
	}

	data, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	var cust api.Customer
	if err := client.Patch(ctx, "/customers/"+c.Customer, body, &cust); err != nil {
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
