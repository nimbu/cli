package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// OrdersListCmd lists orders.
type OrdersListCmd struct {
	Status  string `help:"Filter by status"`
	All     bool   `help:"Fetch all pages"`
	Page    int    `help:"Page number" default:"1"`
	PerPage int    `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *OrdersListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
		return fmt.Errorf("list orders: %w", err)
	}
	if c.Status != "" {
		opts = append(opts, api.WithParam("status", c.Status))
	}

	var orders []api.Order

	if c.All {
		orders, err = api.List[api.Order](ctx, client, "/orders", opts...)
		if err != nil {
			return fmt.Errorf("list orders: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Order](ctx, client, "/orders", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list orders: %w", err)
		}
		orders = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, orders)
	}

	plainFields := []string{"id", "number", "status", "total", "currency"}
	tableFields := []string{"id", "number", "status", "total", "currency", "customer_id"}
	tableHeaders := []string{"ID", "NUMBER", "STATUS", "TOTAL", "CURRENCY", "CUSTOMER"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, orders, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, orders, fields, headers)
}
