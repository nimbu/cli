package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

// ThemeFilesGetCmd gets/downloads a theme file.
type ThemeFilesGetCmd struct {
	Theme  string `arg:"" help:"Theme ID"`
	Path   string `arg:"" help:"File path within theme"`
	Output string `help:"Write file to path instead of stdout" short:"o"`
}

// Run executes the get command.
func (c *ThemeFilesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
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
	content, err := themes.ReadContent(ctx, client, c.Theme, kind, remoteName)
	if err != nil {
		return fmt.Errorf("get theme file: %w", err)
	}
	displayPath := themes.DisplayPath(kind, remoteName)

	// Write to file if output path specified
	if c.Output != "" {
		if err := os.WriteFile(c.Output, content, 0o644); err != nil {
			return fmt.Errorf("write file: %w", err)
		}

		return output.Print(ctx, output.PathPayload(c.Output), []any{c.Output}, func() error {
			_, err := output.Fprintf(ctx, "Written to: %s\n", c.Output)
			return err
		})
	}

	// Output to stdout
	return output.Print(ctx, map[string]any{
		"path":    displayPath,
		"content": base64.StdEncoding.EncodeToString(content),
	}, nil, func() error {
		_, err := os.Stdout.Write(content)
		return err
	})
}
