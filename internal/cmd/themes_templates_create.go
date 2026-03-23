package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeTemplatesCreateCmd creates or updates a template.
type ThemeTemplatesCreateCmd struct {
	Theme       string   `arg:"" help:"Theme ID"`
	Name        string   `arg:"" help:"Template name including extension"`
	File        string   `help:"Read template code from file" short:"f"`
	Code        string   `help:"Template code (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. folder=emails)"`
}

// Run executes the create command.
func (c *ThemeTemplatesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create template"); err != nil {
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
	path := "/themes/" + url.PathEscape(c.Theme) + "/templates"
	if err := client.Post(ctx, path, body, &result, opts...); err != nil {
		return fmt.Errorf("create template: %w", err)
	}

	return output.Print(ctx, result, []any{result.ID, result.Name}, func() error {
		if _, err := output.Fprintf(ctx, "Upserted template: %s\n", result.Name); err != nil {
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
