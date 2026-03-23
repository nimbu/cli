package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// NotificationsCreateCmd creates a notification.
type NotificationsCreateCmd struct {
	File        string   `help:"Read notification JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. slug=order_created, subject=Hello)"`
}

// Run executes the create command.
func (c *NotificationsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create notification"); err != nil {
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
		return err
	}

	var notification api.Notification
	if err := client.Post(ctx, "/notifications", body, &notification); err != nil {
		return fmt.Errorf("create notification: %w", err)
	}

	return output.Print(ctx, notification, []any{notification.ID, notification.Slug, notification.Name}, func() error {
		_, err := output.Fprintf(ctx, "Created notification: %s (%s)\n", notification.Name, notification.ID)
		return err
	})
}
