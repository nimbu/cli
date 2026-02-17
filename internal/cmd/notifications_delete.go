package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// NotificationsDeleteCmd deletes a notification.
type NotificationsDeleteCmd struct {
	Notification string `arg:"" help:"Notification slug or identifier"`
}

// Run executes the delete command.
func (c *NotificationsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete notification"); err != nil {
		return err
	}

	if err := requireForce(flags, "notification "+c.Notification); err != nil {
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

	path := "/notifications/" + url.PathEscape(c.Notification)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete notification: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("notification deleted"))
	}

	if mode.Plain {
		return output.Plain(ctx, c.Notification, "deleted")
	}

	fmt.Printf("Deleted notification: %s\n", c.Notification)
	return nil
}
