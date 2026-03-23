package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// NotificationsGetCmd gets a notification by slug.
type NotificationsGetCmd struct {
	Notification string `arg:"" help:"Notification slug or identifier"`
}

// Run executes the get command.
func (c *NotificationsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var notification api.Notification
	path := "/notifications/" + url.PathEscape(c.Notification)
	if err := client.Get(ctx, path, &notification); err != nil {
		return fmt.Errorf("get notification: %w", err)
	}

	return output.Detail(ctx, notification, []any{notification.ID, notification.Slug, notification.Name, notification.Subject}, []output.Field{
		output.FAlways("ID", notification.ID),
		output.FAlways("Slug", notification.Slug),
		output.FAlways("Name", notification.Name),
		output.F("Description", notification.Description),
		output.FAlways("Subject", notification.Subject),
		output.FAlways("HTML", notification.HTMLEnabled),
	})
}
