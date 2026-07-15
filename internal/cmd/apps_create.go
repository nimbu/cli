package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// AppsCreateCmd creates an OAuth / cloud-code app.
type AppsCreateCmd struct {
	Name        string   `required:"" help:"App name"`
	Description string   `help:"App description"`
	Internal    bool     `negatable:"" default:"true" help:"Create an internal app (callback derived from scopes). Use --no-internal for an external app."`
	Scopes      []string `help:"Install scopes (comma-separated, e.g. read_channels,write_channels); required for internal apps"`
	URL         string   `help:"App URL (external apps only)"`
	CallbackURL string   `name:"callback-url" help:"OAuth callback URL (external apps only)"`
	SDKVersion  string   `name:"sdk-version" help:"Cloud-code SDK version"`
	Install     *bool    `negatable:"" help:"Grant and register the app on the site in one step (default: true for internal apps)"`
	Configure   bool     `help:"After creating, add the app to the local nimbu.yml project config"`
}

// Run executes the create command.
func (c *AppsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create app"); err != nil {
		return err
	}

	scopes := splitRepeatedCSV(c.Scopes)

	if c.Internal {
		if len(scopes) == 0 {
			return fmt.Errorf("internal apps require --scopes (e.g. --scopes read_channels,write_channels)")
		}
		if strings.TrimSpace(c.CallbackURL) != "" {
			return fmt.Errorf("--callback-url cannot be used with an internal app; the callback is derived from scopes (use --no-internal for an external app)")
		}
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	// install defaults to true for internal apps unless explicitly set.
	install := c.Internal
	if c.Install != nil {
		install = *c.Install
	}

	body := map[string]any{
		"name":     c.Name,
		"internal": c.Internal,
		"install":  install,
	}
	if strings.TrimSpace(c.Description) != "" {
		body["description"] = c.Description
	}
	if len(scopes) > 0 {
		body["install_scopes"] = scopes
	}
	if strings.TrimSpace(c.URL) != "" {
		body["url"] = c.URL
	}
	if strings.TrimSpace(c.CallbackURL) != "" {
		body["callback_url"] = c.CallbackURL
	}
	if strings.TrimSpace(c.SDKVersion) != "" {
		body["sdk_version"] = c.SDKVersion
	}

	var app api.App
	if err := client.Post(ctx, "/apps", body, &app); err != nil {
		return fmt.Errorf("create app: %w", err)
	}

	if c.Configure {
		if err := c.configureLocal(flags, site, app); err != nil {
			return err
		}
	}

	return output.Detail(ctx, app, []any{app.Key, app.Name, app.Internal, strings.Join(app.InstallScopes, ","), app.CallbackURL}, []output.Field{
		output.FAlways("Key", app.Key),
		output.FAlways("Name", app.Name),
		output.FAlways("Internal", app.Internal),
		output.F("Scopes", strings.Join(app.InstallScopes, ", ")),
		output.FAlways("Callback", app.CallbackURL),
	})
}

// configureLocal upserts the created app into the local nimbu.yml, when one is
// resolvable. It uses non-interactive defaults (dir=code, glob=**/*.js).
func (c *AppsCreateCmd) configureLocal(flags *RootFlags, site string, app api.App) error {
	projectFile, err := config.FindProjectFile()
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			fmt.Fprintln(os.Stderr, "no nimbu.yml found; skipping --configure")
			return nil
		}
		return err
	}

	id := strings.TrimSpace(app.Key)
	if id == "" {
		id = strings.TrimSpace(app.Name)
	}
	item := config.AppProjectConfig{
		ID:   id,
		Name: strings.ToLower(strings.ReplaceAll(strings.TrimSpace(app.Name), " ", "_")),
		Dir:  filepath.ToSlash("code"),
		Glob: "**/*.js",
		Host: currentAPIHost(flags),
		Site: site,
	}
	if err := config.UpsertProjectApp(projectFile, item); err != nil {
		return fmt.Errorf("write project config: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Configured app %s in %s\n", item.Name, projectFile)
	return nil
}
