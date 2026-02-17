package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeAssetsGetCmd gets an asset.
type ThemeAssetsGetCmd struct {
	Theme string `arg:"" help:"Theme ID"`
	Path  string `arg:"" help:"Asset path"`
}

// Run executes the get command.
func (c *ThemeAssetsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/themes/%s/assets/%s", url.PathEscape(c.Theme), url.PathEscape(c.Path))
	var asset api.ThemeResource
	if err := client.Get(ctx, path, &asset); err != nil {
		return fmt.Errorf("get asset: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, asset)
	}

	if mode.Plain {
		if asset.Code != "" {
			_, err := os.Stdout.WriteString(asset.Code)
			if err != nil {
				return fmt.Errorf("write stdout: %w", err)
			}
			_, err = os.Stdout.WriteString("\n")
			if err != nil {
				return fmt.Errorf("write stdout: %w", err)
			}
			return nil
		}
		return output.Plain(ctx, asset.ID, asset.Name, asset.Path)
	}

	if asset.Code != "" {
		_, err := os.Stdout.WriteString(asset.Code)
		if err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
		_, err = os.Stdout.WriteString("\n")
		if err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
		return nil
	}

	fmt.Printf("ID:       %s\n", asset.ID)
	fmt.Printf("Name:     %s\n", asset.Name)
	fmt.Printf("Path:     %s\n", asset.Path)
	if asset.PublicURL != "" {
		fmt.Printf("Public:   %s\n", asset.PublicURL)
	}
	if !asset.CreatedAt.IsZero() {
		fmt.Printf("Created:  %s\n", asset.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if !asset.UpdatedAt.IsZero() {
		fmt.Printf("Updated:  %s\n", asset.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	return nil
}
