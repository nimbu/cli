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

	var document api.Document[api.Notification]
	path := "/notifications/" + url.PathEscape(c.Notification)
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Put(ctx, path, body, &document, opts...); err != nil {
		return fmt.Errorf("update notification: %w", err)
	}
	notification := document.Value
	display, err := localizedDocumentMap(document, c.Locale)
	if err != nil {
		return fmt.Errorf("project notification locale: %w", err)
	}

	return output.Print(ctx, document, []any{display["id"], display["slug"], display["name"]}, func() error {
		_, err := output.Fprintf(ctx, "Updated notification: %v (%s)\n", display["name"], notification.ID)
		return err
	})
}
