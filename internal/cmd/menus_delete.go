package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/output"
)

// MenusDeleteCmd deletes a menu.
type MenusDeleteCmd struct {
	Menu string `arg:"" help:"Menu ID or handle"`
}

// Run executes the delete command.
func (c *MenusDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("write operations disabled in readonly mode")
	}

	if !flags.Force {
		return fmt.Errorf("delete requires --force flag")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	if err := client.Delete(ctx, "/menus/"+c.Menu, nil); err != nil {
		return fmt.Errorf("delete menu: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("menu deleted"))
	}

	if mode.Plain {
		return output.Plain(ctx, "deleted", c.Menu)
	}

	fmt.Printf("Deleted menu %s\n", c.Menu)
	return nil
}
