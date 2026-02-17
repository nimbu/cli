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

// OrdersUpdateCmd updates an order.
type OrdersUpdateCmd struct {
	Order  string `arg:"" help:"Order ID or number"`
	Status string `help:"Set order status"`
	File   string `help:"Read order JSON from file (use - for stdin)" type:"existingfile"`
}

// Run executes the update command.
func (c *OrdersUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("cannot update order in readonly mode")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var body map[string]any

	// If --status flag is used, create simple body
	if c.Status != "" {
		body = map[string]any{"status": c.Status}
	} else {
		// Read from file or stdin
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

		if err := json.Unmarshal(data, &body); err != nil {
			return fmt.Errorf("parse JSON: %w", err)
		}
	}

	var o api.Order
	if err := client.Patch(ctx, "/orders/"+c.Order, body, &o); err != nil {
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
