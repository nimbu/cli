package cmd

import (
	"context"
	"fmt"
	"net/url"

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
	path := "/customers/" + url.PathEscape(c.Customer)
	if err := client.Get(ctx, path, &cust); err != nil {
		return fmt.Errorf("get customer: %w", err)
	}

	fields := []output.Field{
		output.FAlways("ID", cust.ID),
		output.FAlways("Email", cust.Email),
		output.F("First Name", cust.FirstName),
		output.F("Last Name", cust.LastName),
		output.F("Phone", cust.Phone),
	}
	if !cust.CreatedAt.IsZero() {
		fields = append(fields, output.FAlways("Created", cust.CreatedAt.Format("2006-01-02 15:04:05")))
	}
	if !cust.UpdatedAt.IsZero() {
		fields = append(fields, output.FAlways("Updated", cust.UpdatedAt.Format("2006-01-02 15:04:05")))
	}

	return output.Detail(ctx, cust, []any{cust.ID, cust.Email, cust.FirstName, cust.LastName, cust.Phone}, fields)
}
