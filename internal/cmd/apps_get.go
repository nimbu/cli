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

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, app)
	}

	if mode.Plain {
		return output.Plain(ctx, app.Key, app.Name, app.Domain, app.CallbackURL)
	}

	fmt.Printf("Key:        %s\n", app.Key)
	fmt.Printf("Name:       %s\n", app.Name)
	fmt.Printf("Domain:     %s\n", app.Domain)
	fmt.Printf("Callback:   %s\n", app.CallbackURL)
	if app.SDKVersion != "" {
		fmt.Printf("SDK:        %s\n", app.SDKVersion)
	}
	fmt.Printf("Functions:  %d\n", len(app.Functions))
	fmt.Printf("Routes:     %d\n", len(app.Routes))
	fmt.Printf("Callbacks:  %d\n", len(app.Callbacks))
	fmt.Printf("Jobs:       %d\n", len(app.Jobs))
	fmt.Printf("Schedules:  %d\n", len(app.Schedules))

	return nil
}
