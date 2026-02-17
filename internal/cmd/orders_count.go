package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// OrdersCountCmd counts orders.
type OrdersCountCmd struct {
	Status string `help:"Filter by status"`
}

// Run executes the count command.
func (c *OrdersCountCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	count, err := api.Count(ctx, client, "/orders/count", opts...)
	if err != nil {
		return fmt.Errorf("count orders: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.CountPayload(count))
	}

	if mode.Plain {
		return output.Plain(ctx, count)
	}

	fmt.Printf("Orders: %d\n", count)
	return nil
}
