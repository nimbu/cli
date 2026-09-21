package cmd

import (
	"context"
	"encoding/json"
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
	Outline        bool   `help:"Emit one compact line per editable (paths usable with pages set --path)"`
	Compact        bool   `help:"JSON only: drop editable timestamps/slug/type, redundant translations, and file noise"`
	Draft          bool   `help:"Read the page draft instead of the live page (works with --shape, --outline, --compact)"`
	Page           string `required:"" help:"Page fullpath"`
}

// Run executes the get command.
func (c *PagesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.Shape && c.Outline {
		return fmt.Errorf("--shape and --outline are mutually exclusive")
	}

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

	var page api.PageDocument
	if c.Draft {
		session, draft, err := loadPageDraft(ctx, flags, c.Page, c.Locale)
		if err != nil {
			return err
		}
		page = draftPageDocument(session, draft)
	} else {
		var err error
		page, err = api.GetPageDocument(ctx, client, c.Page, opts...)
		if err != nil {
			return fmt.Errorf("get page: %w", err)
		}
	}

	mode := output.FromContext(ctx)
	if c.Shape || c.Outline {
		if c.Compact {
			_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: --compact is ignored with --shape/--outline\n")
		}
		if len(listRequestedFields(&c.QueryFlags)) > 0 {
			_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: --fields is ignored with --shape/--outline\n")
		}
	}
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

	if c.Outline {
		if c.DownloadAssets != "" {
			_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: --outline ignores --download-assets\n")
		}
		if mode.Plain {
			_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: --outline ignores --plain\n")
		}
		outlineDoc := page
		if c.Locale != "" {
			projected, err := output.ProjectLocale(page, c.Locale)
			if err != nil {
				return fmt.Errorf("project page locale: %w", err)
			}
			outlineDoc = api.PageDocument(projected.(map[string]any))
		}
		entries := api.PageOutline(outlineDoc)
		if mode.JSON {
			return output.JSON(ctx, entries)
		}
		return printPageOutline(ctx, entries)
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

	if !mode.JSON && (c.Compact || len(listRequestedFields(&c.QueryFlags)) > 0) {
		_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: --compact and --fields only apply to --json output\n")
	}

	if mode.JSON {
		doc := page
		if c.Locale != "" {
			projected, err := output.ProjectLocale(doc, c.Locale)
			if err != nil {
				return fmt.Errorf("project page locale: %w", err)
			}
			doc = api.PageDocument(projected.(map[string]any))
		}
		if c.Compact {
			compacted, err := api.PageCompactDocument(doc)
			if err != nil {
				return fmt.Errorf("compact page: %w", err)
			}
			doc = compacted
		}
		if fields := listRequestedFields(&c.QueryFlags); len(fields) > 0 {
			selected, err := api.PageProjectFields(doc, fields)
			if err != nil {
				return fmt.Errorf("pages get: %w", err)
			}
			doc = selected
		}
		return output.JSON(ctx, doc)
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

const pageOutlinePreviewLimit = 120

// pageOutlineDisplayPath is the path shown on an outline line: the full human
// path, or the raw path when no human path can address the entry.
func pageOutlineDisplayPath(entry api.PageOutlineEntry) string {
	if entry.Path != "" {
		return entry.Path
	}
	return entry.RawPath
}

// printPageOutline renders the outline as one indented line per entry. Every
// line carries the full path so it can be copied straight into pages set.
func printPageOutline(ctx context.Context, entries []api.PageOutlineEntry) error {
	for _, entry := range entries {
		indent := strings.Repeat("  ", entry.Depth)
		path := pageOutlineDisplayPath(entry)
		switch entry.Type {
		case api.PageOutlineTypeRepeatable:
			if _, err := output.Fprintf(ctx, "%s%s %s %s\n", indent, path, entry.Slug, entry.ID); err != nil {
				return err
			}
		case api.PageOutlineTypeCanvas:
			if entry.Repeatables == 0 {
				if _, err := output.Fprintf(ctx, "%s%s (canvas): (empty)\n", indent, path); err != nil {
					return err
				}
				continue
			}
			// A top-level canvas is implied by its repeatable lines.
			if entry.Depth == 0 {
				continue
			}
			if _, err := output.Fprintf(ctx, "%s%s (canvas)\n", indent, path); err != nil {
				return err
			}
		default:
			line := fmt.Sprintf("%s%s (%s): %s", indent, path, entry.Type, pageOutlinePreview(entry.Content))
			if _, err := output.Fprintf(ctx, "%s\n", line); err != nil {
				return err
			}
		}
	}
	return nil
}

// pageOutlinePreview renders an editable value as a single truncated line.
func pageOutlinePreview(content any) string {
	return truncatePreview(collapseWhitespace(pageOutlineValueString(content)))
}

func pageOutlineValueString(content any) string {
	switch value := content.(type) {
	case nil:
		return ""
	case string:
		return value
	case map[string]any:
		return pageOutlineObjectString(value)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(encoded)
	}
}

// pageOutlineObjectString prefers a filename for file objects and a label or
// id for references, falling back to compact JSON.
func pageOutlineObjectString(value map[string]any) string {
	if name := stringAny(value["filename"]); name != "" {
		return name
	}
	if url := stringAny(value["url"]); url != "" {
		if idx := strings.LastIndex(url, "/"); idx >= 0 && idx+1 < len(url) {
			return url[idx+1:]
		}
		return url
	}
	label := firstNonBlank(
		stringAny(value["label"]),
		stringAny(value["title"]),
		stringAny(value["name"]),
	)
	id := firstNonBlank(stringAny(value["id"]), stringAny(value["reference_id"]))
	switch {
	case label != "" && id != "":
		return label + " (" + id + ")"
	case label != "":
		return label
	case id != "":
		return id
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

func collapseWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncatePreview(value string) string {
	if value == "" {
		return "(empty)"
	}
	runes := []rune(value)
	if len(runes) <= pageOutlinePreviewLimit {
		return value
	}
	return strings.TrimRight(string(runes[:pageOutlinePreviewLimit]), " ") + "…"
}
