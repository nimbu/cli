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
	Notification string   `required:"" help:"Notification slug or identifier"`
	Locale       string   `help:"Content locale for localized notification fields"`
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
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Put(ctx, path, body, &notification, opts...); err != nil {
		return fmt.Errorf("update notification: %w", err)
	}

	return output.Print(ctx, notification, []any{notification.ID, notification.Slug, notification.Name}, func() error {
		_, err := output.Fprintf(ctx, "Updated notification: %s (%s)\n", notification.Name, notification.ID)
		return err
	})
}
