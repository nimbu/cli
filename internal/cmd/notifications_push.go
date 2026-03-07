package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/notifications"
	"github.com/nimbu/cli/internal/output"
)

// NotificationsPushCmd uploads local notifications from disk.
type NotificationsPushCmd struct {
	Only []string `help:"Notification slugs to push" short:"o"`
}

// Run executes notifications push.
func (c *NotificationsPushCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "push notification templates"); err != nil {
		return err
	}

	projectRoot, _, err := resolveProjectRoot()
	if err != nil {
		return err
	}
	root := notifications.RootPath(projectRoot)

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	result, err := notifications.Push(ctx, client, root, allowedNotificationLocales(ctx, client, site), setFromSlice(c.Only))
	if err != nil {
		return fmt.Errorf("push notifications: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, action := range result.Actions {
			fmt.Printf("%s\t%s\n", action.Action, action.Slug)
		}
		return nil
	}
	for _, action := range result.Actions {
		fmt.Printf("%s %s\n", action.Action, action.Slug)
	}
	fmt.Printf("push complete: %d templates\n", len(result.Actions))
	return nil
}

func allowedNotificationLocales(ctx context.Context, client *api.Client, site string) map[string]struct{} {
	var details api.Site
	if err := client.Get(ctx, "/sites/"+url.PathEscape(site), &details); err == nil {
		return notifications.AllowedLocales(details.Locales)
	}
	return notifications.AllowedLocales(nil)
}

func setFromSlice(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	items := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			items[value] = struct{}{}
		}
	}
	if len(items) == 0 {
		return nil
	}
	return items
}
