package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// MenusCountCmd gets menu count.
type MenusCountCmd struct {
	CountQueryFlags `embed:""`
}

// Run executes the count command.
func (c *MenusCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	opts, err := countRequestOptions(&c.CountQueryFlags, true)
	if err != nil {
		return fmt.Errorf("count menus: %w", err)
	}
	count, err := api.Count(ctx, client, "/menus/count", opts...)
	if err != nil {
		return fmt.Errorf("count menus: %w", err)
	}

	return output.Print(ctx, output.CountPayload(count), []any{count}, func() error {
		_, err := output.Fprintf(ctx, "Menus: %d\n", count)
		return err
	})
}
