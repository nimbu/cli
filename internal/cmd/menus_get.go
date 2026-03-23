package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// MenusGetCmd gets menu details.
type MenusGetCmd struct {
	Menu string `arg:"" help:"Menu slug or handle"`
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

	menu, err := api.GetMenuDocument(ctx, client, c.Menu)
	if err != nil {
		return fmt.Errorf("get menu: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, menu)
	}

	stats := api.MenuStats(menu)
	if mode.Plain {
		return output.Plain(ctx, menu["id"], api.MenuDocumentSlug(menu), api.MenuDocumentName(menu), stats.ItemCount)
	}

	if _, err := output.Fprintf(ctx, "ID:        %v\n", menu["id"]); err != nil {
		return err
	}
	if slug := api.MenuDocumentSlug(menu); slug != "" {
		if _, err := output.Fprintf(ctx, "Slug:      %s\n", slug); err != nil {
			return err
		}
	}
	if handle := api.MenuDocumentHandle(menu); handle != "" {
		if _, err := output.Fprintf(ctx, "Handle:    %s\n", handle); err != nil {
			return err
		}
	}
	if _, err := output.Fprintf(ctx, "Name:      %s\n", api.MenuDocumentName(menu)); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Items:     %d\n", stats.ItemCount); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Max depth: %d\n", stats.MaxDepth); err != nil {
		return err
	}

	return nil
}
