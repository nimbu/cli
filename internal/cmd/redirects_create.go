package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// RedirectsCreateCmd creates a redirect.
type RedirectsCreateCmd struct {
	File        string   `help:"Read redirect JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. source=/old, target=/new)"`
}

// Run executes the create command.
func (c *RedirectsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create redirect"); err != nil {
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
	if err := client.Post(ctx, "/redirects", body, &redirect); err != nil {
		return fmt.Errorf("create redirect: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, redirect)
	}

	if mode.Plain {
		return output.Plain(ctx, redirect.ID, redirect.Source, redirect.Target)
	}

	fmt.Printf("Created redirect: %s -> %s (%s)\n", redirect.Source, redirect.Target, redirect.ID)
	return nil
}
