package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

// ThemeFilesDeleteCmd deletes a theme file.
type ThemeFilesDeleteCmd struct {
	Theme string `required:"" help:"Theme ID"`
	Path  string `required:"" help:"File path within theme"`
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

	kind, remoteName := themes.ParseCLIPath(c.Path)
	if remoteName == "" || remoteName == "." {
		return fmt.Errorf("invalid theme file path: %s", c.Path)
	}
	resource := themes.Resource{
		DisplayPath: themes.DisplayPath(kind, remoteName),
		Kind:        kind,
		RemoteName:  remoteName,
	}
	if err := themes.Delete(ctx, client, c.Theme, resource); err != nil {
		return fmt.Errorf("delete theme file: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload(fmt.Sprintf("deleted %s", resource.DisplayPath)), []any{resource.DisplayPath, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted: %s\n", resource.DisplayPath)
		return err
	})
}
