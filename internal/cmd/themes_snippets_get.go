package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeSnippetsGetCmd gets a snippet.
type ThemeSnippetsGetCmd struct {
	Theme string `arg:"" help:"Theme ID"`
	Name  string `arg:"" help:"Snippet name including extension"`
}

// Run executes the get command.
func (c *ThemeSnippetsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/themes/%s/snippets/%s", url.PathEscape(c.Theme), url.PathEscape(c.Name))
	var snippet api.ThemeResource
	if err := client.Get(ctx, path, &snippet); err != nil {
		return fmt.Errorf("get snippet: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, snippet)
	}

	if mode.Plain {
		if snippet.Code != "" {
			_, err := os.Stdout.WriteString(snippet.Code)
			if err != nil {
				return fmt.Errorf("write stdout: %w", err)
			}
			_, err = os.Stdout.WriteString("\n")
			if err != nil {
				return fmt.Errorf("write stdout: %w", err)
			}
			return nil
		}
		return output.Plain(ctx, snippet.ID, snippet.Name, snippet.UpdatedAt)
	}

	if snippet.Code != "" {
		_, err := os.Stdout.WriteString(snippet.Code)
		if err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
		_, err = os.Stdout.WriteString("\n")
		if err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
		return nil
	}

	fmt.Printf("ID:       %s\n", snippet.ID)
	fmt.Printf("Name:     %s\n", snippet.Name)
	fmt.Printf("URL:      %s\n", snippet.URL)
	fmt.Printf("Permalink:%s\n", snippet.Permalink)
	if !snippet.CreatedAt.IsZero() {
		fmt.Printf("Created:  %s\n", snippet.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if !snippet.UpdatedAt.IsZero() {
		fmt.Printf("Updated:  %s\n", snippet.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	return nil
}
