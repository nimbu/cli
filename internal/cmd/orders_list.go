package cmd

import (
	"context"
	"fmt"
	"strings"

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
	if err := requireScopes(ctx, client, []string{"read_orders"}, "Example: nimbu auth scopes"); err != nil {
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
	var meta listFooterMeta

	if c.All {
		orders, err = api.List[api.Order](ctx, client, "/orders", opts...)
		if err != nil {
			return fmt.Errorf("list orders: %w", err)
		}
		meta = allListFooterMeta(len(orders))
	} else {
		paged, err := api.ListPage[api.Order](ctx, client, "/orders", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list orders: %w", err)
		}
		orders = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(orders))
		meta.probeTotal(ctx, client, "/orders/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, orders)
	}

	displayOrders := buildOrderListRows(orders)

	plainFields := []string{"id", "number", "status", "total", "currency"}
	tableFields := []string{"id", "number", "status", "total", "currency", "customer_id"}
	tableHeaders := []string{"ID", "NUMBER", "STATUS", "TOTAL", "CURRENCY", "CUSTOMER"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, displayOrders, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, displayOrders, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "orders", meta)
}

func buildOrderListRows(orders []api.Order) []api.Order {
	rows := make([]api.Order, len(orders))
	for i := range orders {
		order := orders[i]
		order.Number = orderDisplayNumber(order)
		rows[i] = order
	}
	return rows
}

func orderDisplayNumber(order api.Order) string {
	if strings.TrimSpace(order.Number) != "" {
		return order.Number
	}
	if len(order.ID) >= 8 {
		return order.ID[:8]
	}
	if strings.TrimSpace(order.ID) != "" {
		return order.ID
	}
	return "-"
}
