package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// OrdersListCmd lists orders.
type OrdersListCmd struct {
	Status string `help:"Filter by status"`
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

	var opts []api.RequestOption
	if c.Status != "" {
		opts = append(opts, api.WithParam("status", c.Status))
	}

	orders, err := api.List[api.Order](ctx, client, "/orders", opts...)
	if err != nil {
		return fmt.Errorf("list orders: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, orders)
	}

	if mode.Plain {
		for _, o := range orders {
			if err := output.Plain(ctx, o.ID, o.Number, o.Status, o.Total, o.Currency); err != nil {
				return err
			}
		}
		return nil
	}

	// Human-readable table
	fields := []string{"id", "number", "status", "total", "currency", "customer_id"}
	headers := []string{"ID", "NUMBER", "STATUS", "TOTAL", "CURRENCY", "CUSTOMER"}
	return output.WriteTable(ctx, orders, fields, headers)
}
