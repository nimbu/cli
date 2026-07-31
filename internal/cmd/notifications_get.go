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
	Notification string `required:"" help:"Notification slug or identifier"`
	Locale       string `help:"Content locale for localized notification fields"`
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

	var document api.Document[api.Notification]
	path := "/notifications/" + url.PathEscape(c.Notification)
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Get(ctx, path, &document, opts...); err != nil {
		return fmt.Errorf("get notification: %w", err)
	}
	notification := document.Value
	display, err := localizedDocumentMap(document, c.Locale)
	if err != nil {
		return fmt.Errorf("project notification locale: %w", err)
	}

	return output.Detail(ctx, document, []any{display["id"], display["slug"], display["name"], display["subject"]}, []output.Field{
		output.FAlways("ID", display["id"]),
		output.FAlways("Slug", display["slug"]),
		output.FAlways("Name", display["name"]),
		output.F("Description", display["description"]),
		output.FAlways("Subject", display["subject"]),
		output.FAlways("HTML", notification.HTMLEnabled),
	})
}
