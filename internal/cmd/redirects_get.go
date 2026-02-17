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

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, redirect)
	}

	if mode.Plain {
		return output.Plain(ctx, redirect.ID, redirect.Source, redirect.Target)
	}

	fmt.Printf("ID:     %s\n", redirect.ID)
	fmt.Printf("Source: %s\n", redirect.Source)
	fmt.Printf("Target: %s\n", redirect.Target)

	return nil
}
