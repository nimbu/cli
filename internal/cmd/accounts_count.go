package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// AccountsCountCmd gets count of accounts.
type AccountsCountCmd struct {
	CountQueryFlags `embed:""`
}

// Run executes the count command.
func (c *AccountsCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	opts, err := countRequestOptions(&c.CountQueryFlags, false)
	if err != nil {
		return fmt.Errorf("count accounts: %w", err)
	}
	count, err := api.Count(ctx, client, "/accounts/count", opts...)
	if err != nil {
		return fmt.Errorf("count accounts: %w", err)
	}

	return output.Print(ctx, output.CountPayload(count), []any{count}, func() error {
		_, err := output.Fprintf(ctx, "Accounts: %d\n", count)
		return err
	})
}
