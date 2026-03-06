package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themesync"
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

	kind, remoteName := themesync.ParseCLIPath(c.Path)
	if remoteName == "" || remoteName == "." {
		return fmt.Errorf("invalid theme file path: %s", c.Path)
	}
	content, err := themesync.ReadContent(ctx, client, c.Theme, kind, remoteName)
	if err != nil {
		return fmt.Errorf("get theme file: %w", err)
	}
	displayPath := themesync.DisplayPath(kind, remoteName)

	// Write to file if output path specified
	if c.Output != "" {
		if err := os.WriteFile(c.Output, content, 0o644); err != nil {
			return fmt.Errorf("write file: %w", err)
		}

		mode := output.FromContext(ctx)
		if mode.JSON {
			return output.JSON(ctx, output.PathPayload(c.Output))
		}
		if mode.Plain {
			return output.Plain(ctx, c.Output)
		}
		fmt.Printf("Written to: %s\n", c.Output)
		return nil
	}

	// Output to stdout
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]any{
			"path":    displayPath,
			"content": base64.StdEncoding.EncodeToString(content),
		})
	}

	// Plain and human: write raw content
	_, err = os.Stdout.Write(content)
	return err
}
