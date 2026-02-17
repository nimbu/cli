package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// RedirectsUpdateCmd updates a redirect.
type RedirectsUpdateCmd struct {
	Redirect    string   `arg:"" help:"Redirect ID"`
	File        string   `help:"Read redirect JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. source=/old, target=/new)"`
}

// Run executes the update command.
func (c *RedirectsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update redirect"); err != nil {
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

	var redirect api.Redirect
	path := "/redirects/" + url.PathEscape(c.Redirect)
	if err := client.Put(ctx, path, body, &redirect); err != nil {
		return fmt.Errorf("update redirect: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, redirect)
	}

	if mode.Plain {
		return output.Plain(ctx, redirect.ID, redirect.Source, redirect.Target)
	}

	fmt.Printf("Updated redirect: %s -> %s (%s)\n", redirect.Source, redirect.Target, redirect.ID)
	return nil
}
