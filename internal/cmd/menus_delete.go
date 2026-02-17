package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// MenusDeleteCmd deletes a menu.
type MenusDeleteCmd struct {
	Menu string `arg:"" help:"Menu ID or handle"`
}

// Run executes the delete command.
func (c *MenusDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete menu"); err != nil {
		return err
	}

	if err := requireForce(flags, "menu "+c.Menu); err != nil {
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

	path := "/menus/" + url.PathEscape(c.Menu)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete menu: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("menu deleted"))
	}

	if mode.Plain {
		return output.Plain(ctx, c.Menu, "deleted")
	}

	fmt.Printf("Deleted menu %s\n", c.Menu)
	return nil
}
