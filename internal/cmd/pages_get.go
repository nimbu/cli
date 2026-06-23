package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PagesGetCmd gets page details.
type PagesGetCmd struct {
	QueryFlags     `embed:""`
	DownloadAssets string `help:"Download page file editables into DIR and rewrite JSON to attachment_path refs"`
	Shape          bool   `help:"Emit canvas/repeatable skeleton instead of content"`
	Page           string `required:"" help:"Page fullpath"`
}

// Run executes the get command.
func (c *PagesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithLocale(c.Locale))
	}

	page, err := api.GetPageDocument(ctx, client, c.Page, opts...)
	if err != nil {
		return fmt.Errorf("get page: %w", err)
	}

	mode := output.FromContext(ctx)
	if c.Shape {
		if c.DownloadAssets != "" {
			_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: --shape ignores --download-assets\n")
		}
		if mode.Plain {
			_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: --shape ignores --plain\n")
		}
		shape := api.PageShape(page)
		if mode.JSON {
			return output.JSON(ctx, shape)
		}
		return printPageShape(ctx, shape)
	}

	if c.DownloadAssets != "" {
		_, warnings, err := api.DownloadPageAssets(ctx, client, page, c.DownloadAssets)
		if err != nil {
			return fmt.Errorf("download page assets: %w", err)
		}
		for _, w := range warnings {
			_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: %s\n", w)
		}
	}

	if mode.JSON {
		return output.JSON(ctx, page)
	}

	stats := api.PageStats(page)
	if mode.Plain {
		return output.Plain(
			ctx,
			page["id"],
			api.PageDocumentFullpath(page),
			api.PageDocumentTitle(page),
			api.PageDocumentPublished(page),
		)
	}

	if _, err := output.Fprintf(ctx, "ID:           %v\n", page["id"]); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Fullpath:     %s\n", api.PageDocumentFullpath(page)); err != nil {
		return err
	}
	if parent := api.PageDocumentParentPath(page); parent != "" {
		if _, err := output.Fprintf(ctx, "Parent path:  %s\n", parent); err != nil {
			return err
		}
	}
	if _, err := output.Fprintf(ctx, "Title:        %s\n", api.PageDocumentTitle(page)); err != nil {
		return err
	}
	if template := api.PageDocumentTemplate(page); template != "" {
		if _, err := output.Fprintf(ctx, "Template:     %s\n", template); err != nil {
			return err
		}
	}
	if _, err := output.Fprintf(ctx, "Published:    %v\n", api.PageDocumentPublished(page)); err != nil {
		return err
	}
	if locale := api.PageDocumentLocale(page); locale != "" {
		if _, err := output.Fprintf(ctx, "Locale:       %s\n", locale); err != nil {
			return err
		}
	}
	if _, err := output.Fprintf(ctx, "Editables:    %d\n", stats.EditableCount); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Attachments:  %d\n", stats.AttachmentCount); err != nil {
		return err
	}
	if c.DownloadAssets != "" {
		if _, err := output.Fprintf(ctx, "Assets dir:   %s\n", c.DownloadAssets); err != nil {
			return err
		}
	}
	return nil
}

// printPageShape renders the page skeleton as an indented, sorted tree.
func printPageShape(ctx context.Context, shape any) error {
	root, ok := shape.(map[string]any)
	if !ok {
		return nil
	}
	return writePageShapeItems(ctx, root, 0)
}

func writePageShapeItems(ctx context.Context, items map[string]any, depth int) error {
	indent := strings.Repeat("  ", depth)
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		entry, _ := items[name].(map[string]any)
		typ, _ := entry["type"].(string)
		if _, err := output.Fprintf(ctx, "%s%s (%s)\n", indent, name, typ); err != nil {
			return err
		}
		repeatables, ok := entry["repeatables"].([]any)
		if !ok {
			continue
		}
		for _, raw := range repeatables {
			rep, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			slug, _ := rep["slug"].(string)
			if _, err := output.Fprintf(ctx, "%s  - %s\n", indent, slug); err != nil {
				return err
			}
			if childItems, ok := rep["items"].(map[string]any); ok && len(childItems) > 0 {
				if err := writePageShapeItems(ctx, childItems, depth+2); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
