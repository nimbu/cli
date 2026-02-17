package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// OrdersUpdateCmd updates an order.
type OrdersUpdateCmd struct {
	Order       string   `arg:"" help:"Order ID or number"`
	Status      string   `help:"Set order status"`
	File        string   `help:"Read order JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. status=paid, note=Done)"`
}

// Run executes the update command.
func (c *OrdersUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update order"); err != nil {
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
		if c.Status == "" || !errors.Is(err, errNoJSONInput) {
			return err
		}
		body = map[string]any{}
	}

	if c.Status != "" {
		statusBody, mergeErr := mergeJSONBodies(body, map[string]any{"status": c.Status})
		if mergeErr != nil {
			return fmt.Errorf("merge --status with request body: %w", mergeErr)
		}
		body = statusBody
	}

	var o api.Order
	path := "/orders/" + url.PathEscape(c.Order)
	if err := client.Put(ctx, path, body, &o); err != nil {
		return fmt.Errorf("update order: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, o)
	}

	if mode.Plain {
		return output.Plain(ctx, o.ID, o.Number, o.Status)
	}

	fmt.Printf("Updated order: %s (status: %s)\n", o.Number, o.Status)
	return nil
}
