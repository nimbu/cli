package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeTemplatesGetCmd gets a template.
type ThemeTemplatesGetCmd struct {
	Theme string `arg:"" help:"Theme ID"`
	Name  string `arg:"" help:"Template name including extension"`
}

// Run executes the get command.
func (c *ThemeTemplatesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/themes/%s/templates/%s", url.PathEscape(c.Theme), url.PathEscape(c.Name))
	var tmpl api.ThemeResource
	if err := client.Get(ctx, path, &tmpl); err != nil {
		return fmt.Errorf("get template: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, tmpl)
	}

	if mode.Plain {
		if tmpl.Code != "" {
			_, err := os.Stdout.WriteString(tmpl.Code)
			if err != nil {
				return fmt.Errorf("write stdout: %w", err)
			}
			_, err = os.Stdout.WriteString("\n")
			if err != nil {
				return fmt.Errorf("write stdout: %w", err)
			}
			return nil
		}
		return output.Plain(ctx, tmpl.ID, tmpl.Name, tmpl.UpdatedAt)
	}

	if tmpl.Code != "" {
		_, err := os.Stdout.WriteString(tmpl.Code)
		if err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
		_, err = os.Stdout.WriteString("\n")
		if err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
		return nil
	}

	fmt.Printf("ID:       %s\n", tmpl.ID)
	fmt.Printf("Name:     %s\n", tmpl.Name)
	fmt.Printf("URL:      %s\n", tmpl.URL)
	fmt.Printf("Permalink:%s\n", tmpl.Permalink)
	if !tmpl.CreatedAt.IsZero() {
		fmt.Printf("Created:  %s\n", tmpl.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if !tmpl.UpdatedAt.IsZero() {
		fmt.Printf("Updated:  %s\n", tmpl.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	return nil
}
