package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// NotificationsCountCmd gets count of notifications.
type NotificationsCountCmd struct {
	CountQueryFlags `embed:""`
}

// Run executes the count command.
func (c *NotificationsCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	opts, err := countRequestOptions(&c.CountQueryFlags, false)
	if err != nil {
		return fmt.Errorf("count notifications: %w", err)
	}
	count, err := api.Count(ctx, client, "/notifications/count", opts...)
	if err != nil {
		return fmt.Errorf("count notifications: %w", err)
	}

	return output.Print(ctx, output.CountPayload(count), []any{count}, func() error {
		_, err := output.Fprintf(ctx, "Notifications: %d\n", count)
		return err
	})
}
