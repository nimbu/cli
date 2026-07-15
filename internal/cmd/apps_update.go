package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// AppsUpdateCmd updates an existing app. Only the flags you provide are sent.
type AppsUpdateCmd struct {
	App         string   `required:"" help:"Application ID"`
	Name        *string  `help:"App name"`
	Description *string  `help:"App description"`
	Internal    *bool    `negatable:"" help:"Whether the app is internal (callback derived from scopes)"`
	Scopes      []string `help:"Install scopes (comma-separated, e.g. read_channels,write_channels)"`
	URL         *string  `help:"App URL (external apps only)"`
	CallbackURL *string  `name:"callback-url" help:"OAuth callback URL (external apps only)"`
	SDKVersion  *string  `name:"sdk-version" help:"Cloud-code SDK version"`
}

// Run executes the update command.
func (c *AppsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update app"); err != nil {
		return err
	}

	body := map[string]any{}
	if c.Name != nil {
		body["name"] = *c.Name
	}
	if c.Description != nil {
		body["description"] = *c.Description
	}
	if c.Internal != nil {
		body["internal"] = *c.Internal
	}
	if c.Scopes != nil {
		body["install_scopes"] = splitRepeatedCSV(c.Scopes)
	}
	if c.URL != nil {
		body["url"] = *c.URL
	}
	if c.CallbackURL != nil {
		body["callback_url"] = *c.CallbackURL
	}
	if c.SDKVersion != nil {
		body["sdk_version"] = *c.SDKVersion
	}

	if len(body) == 0 {
		return fmt.Errorf("no fields to update; pass at least one of --name, --description, --internal, --scopes, --url, --callback-url, --sdk-version")
	}

	if c.Internal != nil && *c.Internal && c.CallbackURL != nil && strings.TrimSpace(*c.CallbackURL) != "" {
		return fmt.Errorf("--callback-url cannot be used with an internal app; the callback is derived from scopes")
	}

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
	if err := client.Patch(ctx, path, body, &app); err != nil {
		return fmt.Errorf("update app: %w", err)
	}

	return output.Detail(ctx, app, []any{app.Key, app.Name, app.Internal, strings.Join(app.InstallScopes, ","), app.CallbackURL}, []output.Field{
		output.FAlways("Key", app.Key),
		output.FAlways("Name", app.Name),
		output.FAlways("Internal", app.Internal),
		output.F("Scopes", strings.Join(app.InstallScopes, ", ")),
		output.FAlways("Callback", app.CallbackURL),
	})
}
