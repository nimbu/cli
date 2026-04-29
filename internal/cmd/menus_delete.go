package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// MenusDeleteCmd deletes a menu.
type MenusDeleteCmd struct {
	Menu string `required:"" help:"Menu ID or handle"`
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

	return output.Print(ctx, output.SuccessPayload("menu deleted"), []any{c.Menu, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted menu %s\n", c.Menu)
		return err
	})
}
