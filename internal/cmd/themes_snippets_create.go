package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeSnippetsCreateCmd creates or updates a snippet.
type ThemeSnippetsCreateCmd struct {
	Theme       string   `arg:"" help:"Theme ID"`
	Name        string   `arg:"" help:"Snippet name including extension"`
	File        string   `help:"Read snippet code from file" short:"f"`
	Code        string   `help:"Snippet code (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. folder=shared)"`
}

// Run executes the create command.
func (c *ThemeSnippetsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create snippet"); err != nil {
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

	content, err := readThemeContent(c.File, c.Code)
	if err != nil {
		return fmt.Errorf("read code: %w", err)
	}

	body := map[string]any{
		"name": c.Name,
		"code": string(content),
	}
	if len(c.Assignments) > 0 {
		inlineBody, err := parseInlineAssignments(c.Assignments)
		if err != nil {
			return err
		}
		body, err = mergeJSONBodies(inlineBody, body)
		if err != nil {
			return fmt.Errorf("merge inline assignments: %w", err)
		}
	}

	opts := []api.RequestOption{}
	if flags != nil && flags.Force {
		opts = append(opts, api.WithQuery(map[string]string{"force": "true"}))
	}

	var result api.ThemeResource
	path := "/themes/" + url.PathEscape(c.Theme) + "/snippets"
	if err := client.Post(ctx, path, body, &result, opts...); err != nil {
		return fmt.Errorf("create snippet: %w", err)
	}

	return output.Print(ctx, result, []any{result.ID, result.Name}, func() error {
		if _, err := output.Fprintf(ctx, "Upserted snippet: %s\n", result.Name); err != nil {
			return err
		}
		if result.ID != "" {
			if _, err := output.Fprintf(ctx, "ID: %s\n", result.ID); err != nil {
				return err
			}
		}
		return nil
	})
}
