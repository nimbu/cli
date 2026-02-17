package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CustomersListCmd lists customers.
type CustomersListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

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

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list customers: %w", err)
	}

	var customers []api.Customer

	if c.All {
		customers, err = api.List[api.Customer](ctx, client, "/customers", opts...)
		if err != nil {
			return fmt.Errorf("list customers: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Customer](ctx, client, "/customers", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list customers: %w", err)
		}
		customers = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, customers)
	}

	plainFields := []string{"id", "email", "first_name", "last_name"}
	tableFields := []string{"id", "email", "first_name", "last_name"}
	tableHeaders := []string{"ID", "EMAIL", "FIRST NAME", "LAST NAME"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, customers, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, customers, fields, headers)
}
