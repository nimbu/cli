package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// MenusUpdateCmd updates a menu.
type MenusUpdateCmd struct {
	Menu        string   `arg:"" help:"Menu ID or handle"`
	File        string   `help:"Read menu JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Main, handle=main)"`
}

// Run executes the update command.
func (c *MenusUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update menu"); err != nil {
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

	var menu api.Menu
	path := "/menus/" + url.PathEscape(c.Menu)
	if err := client.Put(ctx, path, body, &menu); err != nil {
		return fmt.Errorf("update menu: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, menu)
	}

	if mode.Plain {
		return output.Plain(ctx, menu.ID)
	}

	fmt.Printf("Updated menu %s\n", menu.ID)
	return nil
}
