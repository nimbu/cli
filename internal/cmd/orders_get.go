package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// OrdersGetCmd gets an order by ID or number.
type OrdersGetCmd struct {
	Order string `arg:"" help:"Order ID or number"`
}

// Run executes the get command.
func (c *OrdersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var o api.Order
	path := "/orders/" + url.PathEscape(c.Order)
	if err := client.Get(ctx, path, &o); err != nil {
		return fmt.Errorf("get order: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, o)
	}

	if mode.Plain {
		return output.Plain(ctx, o.ID, o.Number, o.Status, o.Total, o.Currency, o.CustomerID)
	}

	fmt.Printf("ID:         %s\n", o.ID)
	fmt.Printf("Number:     %s\n", o.Number)
	fmt.Printf("Status:     %s\n", o.Status)
	fmt.Printf("Total:      %.2f %s\n", o.Total, o.Currency)
	if o.CustomerID != "" {
		fmt.Printf("Customer:   %s\n", o.CustomerID)
	}
	if !o.CreatedAt.IsZero() {
		fmt.Printf("Created:    %s\n", o.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if !o.UpdatedAt.IsZero() {
		fmt.Printf("Updated:    %s\n", o.UpdatedAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}
