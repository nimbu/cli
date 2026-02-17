package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemesGetCmd gets theme details.
type ThemesGetCmd struct {
	Theme string `arg:"" help:"Theme ID"`
}

// Run executes the get command.
func (c *ThemesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var theme api.Theme
	if err := client.Get(ctx, "/themes/"+c.Theme, &theme); err != nil {
		return fmt.Errorf("get theme: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, theme)
	}

	if mode.Plain {
		return output.Plain(ctx, theme.ID, theme.Name, theme.Active)
	}

	fmt.Printf("ID:     %s\n", theme.ID)
	fmt.Printf("Name:   %s\n", theme.Name)
	fmt.Printf("Active: %v\n", theme.Active)
	if !theme.CreatedAt.IsZero() {
		fmt.Printf("Created: %s\n", theme.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if !theme.UpdatedAt.IsZero() {
		fmt.Printf("Updated: %s\n", theme.UpdatedAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}
