package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// AppsGetCmd gets an app by ID.
type AppsGetCmd struct {
	App string `arg:"" help:"Application ID"`
}

// Run executes the get command.
func (c *AppsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var app api.App
	path := "/apps/" + url.PathEscape(c.App)
	if err := client.Get(ctx, path, &app); err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	return output.Detail(ctx, app, []any{app.Key, app.Name, app.Domain, app.CallbackURL}, []output.Field{
		output.FAlways("Key", app.Key),
		output.FAlways("Name", app.Name),
		output.FAlways("Domain", app.Domain),
		output.FAlways("Callback", app.CallbackURL),
		output.F("SDK", app.SDKVersion),
		output.FAlways("Functions", len(app.Functions)),
		output.FAlways("Routes", len(app.Routes)),
		output.FAlways("Callbacks", len(app.Callbacks)),
		output.FAlways("Jobs", len(app.Jobs)),
		output.FAlways("Schedules", len(app.Schedules)),
	})
}
