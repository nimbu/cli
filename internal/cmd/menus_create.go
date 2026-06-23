package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// MenusCreateCmd creates a menu.
type MenusCreateCmd struct {
	File        string   `help:"Read menu JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Main, handle=main)"`
}

// Run executes the create command.
func (c *MenusCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create menu"); err != nil {
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

	var body api.MenuDocument
	if c.File != "" {
		if len(c.Assignments) > 0 {
			return fmt.Errorf("use either --file or inline assignments, not both")
		}
		rawBody, err := readRichDocumentInput(c.File)
		if err != nil {
			return err
		}
		body = api.MenuDocument(rawBody)
	} else {
		rawBody, err := readJSONBodyInput("", c.Assignments)
		if err != nil {
			return err
		}
		body = api.MenuDocument(rawBody)
	}
	api.NormalizeMenuDocumentForWrite(body)

	menu, err := api.PostMenuDocument(ctx, client, body)
	if err != nil {
		return fmt.Errorf("create menu: %w", err)
	}

	return output.Print(ctx, menu, []any{menu["id"]}, func() error {
		_, err := output.Fprintf(ctx, "Created menu %v\n", menu["id"])
		return err
	})
}
