package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PagesUpdateCmd updates a page.
type PagesUpdateCmd struct {
	QueryFlags       `embed:""`
	Page             string   `required:"" help:"Page fullpath"`
	File             string   `help:"Read page JSON from file (use - for stdin)"`
	Replace          bool     `help:"Replace canvases entirely (destructive rebuild)"`
	AllowEmptyCanvas bool     `help:"Allow emptying a canvas"`
	AllowEmptyFile   bool     `help:"Allow file editables with no attachment or URL to clear assets"`
	DryRun           bool     `name:"dry-run" help:"Fetch, merge, and print the PATCH body without sending it"`
	Assignments      []string `arg:"" optional:"" help:"Inline assignments (e.g. title=About, published:=true)"`
}

// Run executes the update command.
func (c *PagesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "update page"); err != nil {
			return err
		}
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
	if c.Replace && c.File == "" {
		return fmt.Errorf("--replace requires --file")
	}
	if c.Replace && len(c.Assignments) > 0 {
		return fmt.Errorf("--replace requires --file; inline assignments use merge semantics")
	}

	var body api.PageDocument
	var current api.PageDocument
	if len(c.Assignments) > 0 {
		if err := validateShallowInlineAssignments("pages update", c.Assignments, map[string]struct{}{
			"title":        {},
			"template":     {},
			"published":    {},
			"locale":       {},
			"translations": {},
		}); err != nil {
			return err
		}

		// Fetch for existence, canvas guards, and the 422 template hint only.
		// The fetched document is never merged into the PATCH body: the API
		// resolves every locale with fallbacks (an untranslated `en` echoes
		// the `nl` title, slug, ...) and processes `translations` after the
		// top-level fields, so writing it back materializes fallback copies
		// and silently reverts localized slugs.
		current, err = api.GetPageDocument(ctx, client, c.Page, opts...)
		if err != nil {
			return fmt.Errorf("fetch current page: %w", err)
		}

		updates, err := readJSONBodyInput("", c.Assignments)
		if err != nil {
			return err
		}
		body = api.PageDocument(updates)
	} else {
		rawBody, err := readRichDocumentInput(c.File)
		if err != nil {
			return err
		}
		body = api.PageDocument(rawBody)
	}

	if err := api.ExpandPageAttachmentPathsWithOptions(body, pageAttachmentFileRefOptions(ctx, client, api.PageAttachmentExpansionOptions{
		AllowEmptyFile:      c.AllowEmptyFile,
		DropEmptyFile:       len(c.Assignments) > 0,
		DropReadOnlyFileURL: !c.Replace,
	})); err != nil {
		return err
	}

	api.NormalizePageDocumentForWrite(body)

	submitted := api.PageStats(body)

	bodyCanvasCounts := api.PageCanvasRepeatableCounts(body)
	if c.Replace || len(bodyCanvasCounts) > 0 {
		if current == nil {
			current, err = api.GetPageDocument(ctx, client, c.Page, opts...)
			if err != nil {
				return fmt.Errorf("fetch current page: %w", err)
			}
		}
		if c.Replace {
			if err := guardCanvasWipe(current, body, bodyCanvasCounts, c.AllowEmptyCanvas); err != nil {
				return err
			}
			opts = append(opts, api.WithReplace(true))
		} else if err := guardExplicitCanvasWipe(current, body, bodyCanvasCounts, c.AllowEmptyCanvas); err != nil {
			return err
		}
	}

	if c.DryRun {
		// TODO: when the pages API grows a validation endpoint, send this body
		// with ?dry_run=1 (api.WithQuery("dry_run", "1")) instead of skipping the write.
		return printPagesUpdateDryRun(ctx, body)
	}

	page, err := api.PatchPageDocument(ctx, client, c.Page, body, opts...)
	if err != nil {
		template := api.PageDocumentTemplate(body)
		if template == "" {
			template = api.PageDocumentTemplate(current)
		}
		return withPageThemeHint(fmt.Errorf("update page: %w", err), template)
	}

	returned := api.PageStats(page)
	if c.File != "" && c.Replace && returned.EditableCount < submitted.EditableCount {
		_, _ = fmt.Fprintf(
			output.WriterFromContext(ctx).Err,
			"warning: server applied %d editables but %d were submitted\n",
			returned.EditableCount, submitted.EditableCount,
		)
	}

	return output.Print(ctx, page, []any{page["id"]}, func() error {
		_, err := output.Fprintf(
			ctx,
			"Updated page %v (%d %s, %d %s)\n",
			page["id"],
			returned.EditableCount,
			pluralize(returned.EditableCount, "editable"),
			returned.AttachmentCount,
			pluralize(returned.AttachmentCount, "attachment"),
		)
		return err
	})
}

// guardCanvasWipe refuses a destructive replace that would empty a populated
// canvas unless the caller explicitly allows it.
func guardCanvasWipe(current, body api.PageDocument, bodyCounts map[string]int, allowEmpty bool) error {
	return guardCanvasWipeMode(current, body, bodyCounts, allowEmpty, true)
}

// guardExplicitCanvasWipe refuses merge updates that explicitly set an existing
// canvas to empty. Omitted canvases are allowed in merge mode.
func guardExplicitCanvasWipe(current, body api.PageDocument, bodyCounts map[string]int, allowEmpty bool) error {
	return guardCanvasWipeMode(current, body, bodyCounts, allowEmpty, false)
}

func guardCanvasWipeMode(current, body api.PageDocument, bodyCounts map[string]int, allowEmpty bool, omissionCountsAsWipe bool) error {
	if allowEmpty {
		return nil
	}
	currentCounts := api.PageCanvasRepeatableCounts(current)
	for _, name := range sortedCanvasPaths(currentCounts) {
		before := currentCounts[name]
		after, present := bodyCounts[name]
		if before > 0 && (present || omissionCountsAsWipe) && after == 0 {
			return fmt.Errorf(
				"refusing to update: canvas '%s' would be wiped from %d->0; pass --allow-empty-canvas to override",
				name, before,
			)
		}
	}

	currentInstanceCounts := api.PageCanvasRepeatableInstanceCounts(current)
	bodyInstanceCounts := api.PageCanvasRepeatableInstanceCounts(body)
	for _, name := range sortedCanvasPaths(currentInstanceCounts) {
		before := currentInstanceCounts[name]
		after, present := bodyInstanceCounts[name]
		if before > 0 && present && after == 0 {
			return fmt.Errorf(
				"refusing to update: canvas '%s' would be wiped from %d->0; pass --allow-empty-canvas to override",
				name, before,
			)
		}
	}
	return nil
}

func sortedCanvasPaths(counts map[string]int) []string {
	paths := make([]string, 0, len(counts))
	for path := range counts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func pluralize(count int, singular string) string {
	if count == 1 {
		return singular
	}
	return singular + "s"
}

func printPagesUpdateDryRun(ctx context.Context, body api.PageDocument) error {
	_, _ = fmt.Fprintln(output.WriterFromContext(ctx).Err, "dry-run: no request sent (the API has no validation endpoint yet)")
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, map[string]any{
			"dry_run": true,
			"body":    body,
		})
	}
	encoded, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dry-run body: %w", err)
	}
	_, err = output.Fprintf(ctx, "%s\n", encoded)
	return err
}
