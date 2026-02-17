package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeFilesListCmd lists theme files.
type ThemeFilesListCmd struct {
	Theme string `arg:"" help:"Theme ID"`
}

// Run executes the list command.
func (c *ThemeFilesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/themes/%s/files", c.Theme)
	files, err := api.List[api.ThemeFile](ctx, client, path)
	if err != nil {
		return fmt.Errorf("list theme files: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, files)
	}

	if mode.Plain {
		for _, f := range files {
			if err := output.Plain(ctx, f.Path, f.Type, f.Size); err != nil {
				return err
			}
		}
		return nil
	}

	fields := []string{"path", "type", "size"}
	headers := []string{"PATH", "TYPE", "SIZE"}
	return output.WriteTable(ctx, files, fields, headers)
}
