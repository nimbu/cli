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

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, notification)
	}

	if mode.Plain {
		return output.Plain(ctx, notification.ID, notification.Slug, notification.Name, notification.Subject)
	}

	fmt.Printf("ID:          %s\n", notification.ID)
	fmt.Printf("Slug:        %s\n", notification.Slug)
	fmt.Printf("Name:        %s\n", notification.Name)
	if notification.Description != "" {
		fmt.Printf("Description: %s\n", notification.Description)
	}
	fmt.Printf("Subject:     %s\n", notification.Subject)
	fmt.Printf("HTML:        %v\n", notification.HTMLEnabled)

	return nil
}
