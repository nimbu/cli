package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// MenusGetCmd gets menu details.
type MenusGetCmd struct {
	Menu string `arg:"" help:"Menu ID or handle"`
}

// Run executes the get command.
func (c *MenusGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var menu api.Menu
	path := "/menus/" + url.PathEscape(c.Menu)
	if err := client.Get(ctx, path, &menu); err != nil {
		return fmt.Errorf("get menu: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, menu)
	}

	if mode.Plain {
		return output.Plain(ctx, menu.ID, menu.Handle, menu.Name, len(menu.Items))
	}

	fmt.Printf("ID:     %s\n", menu.ID)
	fmt.Printf("Handle: %s\n", menu.Handle)
	fmt.Printf("Name:   %s\n", menu.Name)
	fmt.Printf("Items:  %d\n", len(menu.Items))

	return nil
}
