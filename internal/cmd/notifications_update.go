package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// NotificationsUpdateCmd updates a notification.
type NotificationsUpdateCmd struct {
	Notification string   `arg:"" help:"Notification slug or identifier"`
	File         string   `help:"Read notification JSON from file (use - for stdin)"`
	Assignments  []string `arg:"" optional:"" help:"Inline assignments (e.g. subject=Hello, html_enabled:=true)"`
}

// Run executes the update command.
func (c *NotificationsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update notification"); err != nil {
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
	path := "/notifications/" + url.PathEscape(c.Notification)
	if err := client.Put(ctx, path, body, &notification); err != nil {
		return fmt.Errorf("update notification: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, notification)
	}

	if mode.Plain {
		return output.Plain(ctx, notification.ID, notification.Slug, notification.Name)
	}

	fmt.Printf("Updated notification: %s (%s)\n", notification.Name, notification.ID)
	return nil
}
