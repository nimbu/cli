package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeFilesPutCmd uploads/updates a theme file.
type ThemeFilesPutCmd struct {
	Theme   string `arg:"" help:"Theme ID"`
	Path    string `arg:"" help:"File path within theme"`
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

	var content []byte

	// Read from file or use provided content
	switch {
	case c.File != "":
		f, err := os.Open(c.File)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}
		defer func() { _ = f.Close() }()
		content, err = io.ReadAll(f)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	case c.Content != "":
		content = []byte(c.Content)
	default:
		// Read from stdin
		content, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	}

	// Build request body
	body := map[string]any{
		"path":    c.Path,
		"content": base64.StdEncoding.EncodeToString(content),
	}

	path := fmt.Sprintf("/themes/%s/files/%s", url.PathEscape(c.Theme), url.PathEscape(c.Path))
	var result api.ThemeFile
	if err := client.Put(ctx, path, body, &result); err != nil {
		return fmt.Errorf("put theme file: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}

	if mode.Plain {
		return output.Plain(ctx, result.Path)
	}

	fmt.Printf("Uploaded: %s\n", result.Path)
	return nil
}
