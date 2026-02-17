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
	if err := requireWrite(flags, "delete theme file"); err != nil {
		return err
	}

	if err := requireForce(flags, "theme file "+c.Path); err != nil {
		return err
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/themes/%s/files/%s", url.PathEscape(c.Theme), url.PathEscape(c.Path))
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete theme file: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload(fmt.Sprintf("deleted %s", c.Path)))
	}

	if mode.Plain {
		return output.Plain(ctx, c.Path, "deleted")
	}

	fmt.Printf("Deleted: %s\n", c.Path)
	return nil
}
