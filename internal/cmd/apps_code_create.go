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
	App         string   `arg:"" help:"Application ID"`
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

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, file)
	}

	if mode.Plain {
		return output.Plain(ctx, file.Name, file.URL)
	}

	fmt.Printf("Created app code file: %s\n", file.Name)
	return nil
}
