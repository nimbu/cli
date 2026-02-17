package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemesListCmd lists themes.
type ThemesListCmd struct{}

// Run executes the list command.
func (c *ThemesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	themes, err := api.List[api.Theme](ctx, client, "/themes")
	if err != nil {
		return fmt.Errorf("list themes: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, themes)
	}

	if mode.Plain {
		for _, t := range themes {
			if err := output.Plain(ctx, t.ID, t.Name, t.Active); err != nil {
				return err
			}
		}
		return nil
	}

	fields := []string{"id", "name", "active"}
	headers := []string{"ID", "NAME", "ACTIVE"}
	return output.WriteTable(ctx, themes, fields, headers)
}
