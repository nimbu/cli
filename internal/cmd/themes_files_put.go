package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

// ThemeFilesPutCmd uploads/updates a theme file.
type ThemeFilesPutCmd struct {
	Theme   string `required:"" help:"Theme ID"`
	Path    string `required:"" help:"File path within theme"`
	File    string `help:"Read content from file path" short:"f"`
	Content string `help:"File content (base64 for binary)" short:"c"`
}

// Run executes the put command.
func (c *ThemeFilesPutCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update theme file"); err != nil {
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
	content, err := readThemeContent(c.File, c.Content)
	if err != nil {
		return fmt.Errorf("read theme file content: %w", err)
	}
	resource := themes.Resource{
		DisplayPath: themes.DisplayPath(kind, remoteName),
		Kind:        kind,
		RemoteName:  remoteName,
	}
	if err := themes.UpsertBytes(ctx, client, c.Theme, resource, content, flags != nil && flags.Force); err != nil {
		return fmt.Errorf("put theme file: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, resource)
	}

	if mode.Plain {
		return output.Plain(ctx, resource.DisplayPath)
	}

	if _, err := output.Fprintf(ctx, "Uploaded: %s\n", resource.DisplayPath); err != nil {
		return err
	}
	return nil
}
