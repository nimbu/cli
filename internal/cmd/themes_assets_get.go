package cmd

import (
	"context"
	"fmt"
	"net/url"

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

	if asset.Code != "" {
		return output.Print(ctx, asset, []any{asset.Code}, func() error {
			_, err := output.Fprintf(ctx, "%s\n", asset.Code)
			return err
		})
	}

	var created, updated string
	if !asset.CreatedAt.IsZero() {
		created = asset.CreatedAt.Format("2006-01-02 15:04:05")
	}
	if !asset.UpdatedAt.IsZero() {
		updated = asset.UpdatedAt.Format("2006-01-02 15:04:05")
	}

	return output.Detail(ctx, asset, []any{asset.ID, asset.Name, asset.Path}, []output.Field{
		output.FAlways("ID", asset.ID),
		output.FAlways("Name", asset.Name),
		output.FAlways("Path", asset.Path),
		output.F("Public", asset.PublicURL),
		output.F("Created", created),
		output.F("Updated", updated),
	})
}
