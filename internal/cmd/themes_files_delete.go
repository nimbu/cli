package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// ThemeFilesDeleteCmd deletes a theme file.
type ThemeFilesDeleteCmd struct {
	Theme string `arg:"" help:"Theme ID"`
	Path  string `arg:"" help:"File path within theme"`
}

// Run executes the delete command.
func (c *ThemeFilesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("write operations disabled in readonly mode")
	}

	if !flags.Force {
		return fmt.Errorf("delete requires --force flag")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/themes/%s/files/%s", c.Theme, url.PathEscape(c.Path))
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete theme file: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload(fmt.Sprintf("deleted %s", c.Path)))
	}

	if mode.Plain {
		return output.Plain(ctx, "deleted", c.Path)
	}

	fmt.Printf("Deleted: %s\n", c.Path)
	return nil
}
