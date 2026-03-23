package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// RedirectsGetCmd gets a redirect by ID.
type RedirectsGetCmd struct {
	Redirect string `arg:"" help:"Redirect ID"`
}

// Run executes the get command.
func (c *RedirectsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var redirect api.Redirect
	path := "/redirects/" + url.PathEscape(c.Redirect)
	if err := client.Get(ctx, path, &redirect); err != nil {
		return fmt.Errorf("get redirect: %w", err)
	}

	return output.Detail(ctx, redirect, []any{redirect.ID, redirect.Source, redirect.Target}, []output.Field{
		output.FAlways("ID", redirect.ID),
		output.FAlways("Source", redirect.Source),
		output.FAlways("Target", redirect.Target),
	})
}
