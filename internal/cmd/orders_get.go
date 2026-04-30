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
	Order string `required:"" help:"Order ID or number"`
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

	fields := []output.Field{
		output.FAlways("ID", o.ID),
		output.FAlways("Number", o.Number),
		output.FAlways("Status", o.Status),
		output.FAlways("Total", fmt.Sprintf("%.2f %s", o.Total, o.Currency)),
		output.F("Customer", o.CustomerID),
	}
	if !o.CreatedAt.IsZero() {
		fields = append(fields, output.FAlways("Created", o.CreatedAt.Format("2006-01-02 15:04:05")))
	}
	if !o.UpdatedAt.IsZero() {
		fields = append(fields, output.FAlways("Updated", o.UpdatedAt.Format("2006-01-02 15:04:05")))
	}

	return output.Detail(ctx, o, []any{o.ID, o.Number, o.Status, o.Total, o.Currency, o.CustomerID}, fields)
}
