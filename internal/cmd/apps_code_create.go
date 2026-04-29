package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// AppsCodeCreateCmd creates an app code file.
type AppsCodeCreateCmd struct {
	App         string   `required:"" help:"Application ID"`
	File        string   `help:"Read code file JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=main.js, code=console.log(1))"`
}

// Run executes the create command.
func (c *AppsCodeCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create app code file"); err != nil {
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

	var file api.AppCodeFile
	path := "/apps/" + url.PathEscape(c.App) + "/code"
	if err := client.Post(ctx, path, body, &file); err != nil {
		return fmt.Errorf("create app code file: %w", err)
	}

	return output.Print(ctx, file, []any{file.Name, file.URL}, func() error {
		_, err := output.Fprintf(ctx, "Created app code file: %s\n", file.Name)
		return err
	})
}
