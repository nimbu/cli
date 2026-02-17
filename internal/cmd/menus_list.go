package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// MenusListCmd lists menus.
type MenusListCmd struct{}

// Run executes the list command.
func (c *MenusListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	menus, err := api.List[api.Menu](ctx, client, "/menus")
	if err != nil {
		return fmt.Errorf("list menus: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, menus)
	}

	if mode.Plain {
		for _, m := range menus {
			if err := output.Plain(ctx, m.ID, m.Handle, m.Name); err != nil {
				return err
			}
		}
		return nil
	}

	fields := []string{"id", "handle", "name"}
	headers := []string{"ID", "HANDLE", "NAME"}
	return output.WriteTable(ctx, menus, fields, headers)
}
