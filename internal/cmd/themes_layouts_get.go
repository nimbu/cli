package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeLayoutsGetCmd gets a layout.
type ThemeLayoutsGetCmd struct {
	Theme string `arg:"" help:"Theme ID"`
	Name  string `arg:"" help:"Layout name including extension"`
}

// Run executes the get command.
func (c *ThemeLayoutsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/themes/%s/layouts/%s", url.PathEscape(c.Theme), url.PathEscape(c.Name))
	var layout api.ThemeResource
	if err := client.Get(ctx, path, &layout); err != nil {
		return fmt.Errorf("get layout: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, layout)
	}

	if mode.Plain {
		if layout.Code != "" {
			_, err := os.Stdout.WriteString(layout.Code)
			if err != nil {
				return fmt.Errorf("write stdout: %w", err)
			}
			_, err = os.Stdout.WriteString("\n")
			if err != nil {
				return fmt.Errorf("write stdout: %w", err)
			}
			return nil
		}
		return output.Plain(ctx, layout.ID, layout.Name, layout.UpdatedAt)
	}

	if layout.Code != "" {
		_, err := os.Stdout.WriteString(layout.Code)
		if err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
		_, err = os.Stdout.WriteString("\n")
		if err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
		return nil
	}

	fmt.Printf("ID:       %s\n", layout.ID)
	fmt.Printf("Name:     %s\n", layout.Name)
	fmt.Printf("URL:      %s\n", layout.URL)
	fmt.Printf("Permalink:%s\n", layout.Permalink)
	if !layout.CreatedAt.IsZero() {
		fmt.Printf("Created:  %s\n", layout.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if !layout.UpdatedAt.IsZero() {
		fmt.Printf("Updated:  %s\n", layout.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	return nil
}
