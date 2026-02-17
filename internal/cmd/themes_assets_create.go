package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeAssetsCreateCmd creates or updates an asset.
type ThemeAssetsCreateCmd struct {
	Theme       string   `arg:"" help:"Theme ID"`
	Name        string   `arg:"" help:"Asset name or path"`
	File        string   `help:"Read asset content from file" short:"f"`
	ContentType string   `help:"Asset content type"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. content_type=text/css)"`
}

// Run executes the create command.
func (c *ThemeAssetsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create asset"); err != nil {
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

	content, err := readThemeContent(c.File, "")
	if err != nil {
		return fmt.Errorf("read asset content: %w", err)
	}

	body := map[string]any{
		"name":       c.Name,
		"plain_text": string(content),
	}
	if c.ContentType != "" {
		body["content_type"] = c.ContentType
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
	path := "/themes/" + url.PathEscape(c.Theme) + "/assets"
	if err := client.Post(ctx, path, body, &result, opts...); err != nil {
		return fmt.Errorf("create asset: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		if result.Name != "" {
			return output.Plain(ctx, result.Name)
		}
		return output.Plain(ctx, c.Name)
	}

	if result.Name != "" {
		fmt.Printf("Upserted asset: %s\n", result.Name)
		return nil
	}
	fmt.Printf("Upserted asset: %s\n", c.Name)
	return nil
}
