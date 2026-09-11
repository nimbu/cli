package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/pagepath"
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
		opts = append(opts, api.WithContentLocale(c.Locale))
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
		shape := api.PageShapeWithSchema(page, loadPageShapeSchema(ctx, flags, client, page))
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
	projected, err := output.ProjectLocale(page, c.Locale)
	if err != nil {
		return fmt.Errorf("project page locale: %w", err)
	}
	displayPage := api.PageDocument(projected.(map[string]any))

	stats := api.PageStats(displayPage)
	if mode.Plain {
		return output.Plain(
			ctx,
			displayPage["id"],
			api.PageDocumentFullpath(displayPage),
			api.PageDocumentTitle(displayPage),
			api.PageDocumentPublished(displayPage),
		)
	}

	if _, err := output.Fprintf(ctx, "ID:           %v\n", displayPage["id"]); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Fullpath:     %s\n", api.PageDocumentFullpath(displayPage)); err != nil {
		return err
	}
	if parent := api.PageDocumentParentPath(displayPage); parent != "" {
		if _, err := output.Fprintf(ctx, "Parent path:  %s\n", parent); err != nil {
			return err
		}
	}
	if _, err := output.Fprintf(ctx, "Title:        %s\n", api.PageDocumentTitle(displayPage)); err != nil {
		return err
	}
	if template := api.PageDocumentTemplate(displayPage); template != "" {
		if _, err := output.Fprintf(ctx, "Template:     %s\n", template); err != nil {
			return err
		}
	}
	if _, err := output.Fprintf(ctx, "Published:    %v\n", api.PageDocumentPublished(displayPage)); err != nil {
		return err
	}
	if locale := api.PageDocumentLocale(displayPage); locale != "" {
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
		if options, ok := entry["options"].([]string); ok && len(options) > 0 {
			typ = typ + ": " + strings.Join(options, " | ")
		}
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
			id, _ := rep["id"].(string)
			if _, err := output.Fprintf(ctx, "%s  - %s [%v] %s\n", indent, slug, rep["position"], id); err != nil {
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

func loadPageShapeSchema(ctx context.Context, flags *RootFlags, client *api.Client, page api.PageDocument) *pagepath.Schema {
	id, _ := page["id"].(string)
	if strings.TrimSpace(id) == "" {
		return nil
	}
	schema, err := client.GetPageSchema(ctx, id)
	if err == nil {
		return schema
	}
	if flags != nil && flags.Verbose {
		_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: could not fetch page schema: %v\n", err)
	}
	return nil
}
