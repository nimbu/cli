package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PagesDraftCmd groups page draft subcommands.
type PagesDraftCmd struct {
	Get        PagesDraftGetCmd        `cmd:"" help:"Get the current page draft"`
	Save       PagesDraftSaveCmd       `cmd:"" help:"Save a whole page draft from a file"`
	Batch      PagesDraftBatchCmd      `cmd:"" help:"Apply operations to the page draft"`
	Publish    PagesDraftPublishCmd    `cmd:"" help:"Publish a page draft to the live page"`
	Discard    PagesDraftDiscardCmd    `cmd:"" help:"Discard a page draft"`
	PreviewURL PagesDraftPreviewURLCmd `cmd:"" name:"preview-url" help:"Print a preview URL for the page draft"`
}

// PagesDraftGetCmd fetches GET /pages/{id}/draft.
type PagesDraftGetCmd struct {
	Page   string `required:"" help:"Page fullpath or id"`
	Locale string `help:"Content locale for localized fields"`
	Shape  bool   `help:"Emit canvas/repeatable skeleton instead of content"`
	Raw    bool   `help:"Emit the raw draft envelope (content.page_items) instead of the page document"`
}

// Run executes pages draft get.
func (c *PagesDraftGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	session, draft, err := loadPageDraft(ctx, flags, c.Page, c.Locale)
	if err != nil {
		return err
	}
	mode := output.FromContext(ctx)
	if c.Raw {
		if c.Shape {
			return fmt.Errorf("--raw and --shape cannot be combined")
		}
		if mode.JSON {
			return output.JSON(ctx, draft)
		}
		return printDraftRawSummary(ctx, session, draft)
	}

	doc := draftPageDocument(session, draft)
	if c.Shape {
		shape := api.PageShapeWithSchema(doc, loadPageShapeSchema(ctx, flags, session.client, doc))
		if mode.JSON {
			return output.JSON(ctx, shape)
		}
		return printPageShape(ctx, shape)
	}

	if mode.JSON {
		return output.JSON(ctx, map[string]any(doc))
	}
	return printDraftDocumentSummary(ctx, doc, draft)
}

// draftPageDocument renders a draft as a page document: the live page document
// with the draft's snapshot content (page_items) folded in as an items map, plus
// draft metadata under the "draft" key.
func draftPageDocument(session *surgicalSession, draft *api.PageDraft) api.PageDocument {
	base := cloneMap(session.doc)
	if base == nil {
		base = map[string]any{}
	}
	content := draft.ContentMap()
	itemsSource := "draft"
	if converted, ok := draftSnapshotToDocument(content); ok {
		base = mergeDraftResolutionDoc(base, converted)
	} else {
		// The snapshot could not be decoded, so the items below are the LIVE
		// page's. Say so instead of printing live content under a draft header.
		itemsSource = "live"
		warnDraftItemsFallback(session)
		for _, key := range draftSnapshotPageFields {
			if value, exists := content[key]; exists {
				base[key] = value
			}
		}
	}
	if reserved := strings.TrimSpace(draft.ReservedFullpath); reserved != "" {
		base["fullpath"] = reserved
	}
	base["draft"] = map[string]any{
		"id":                draft.ID,
		"page_id":           draft.PageID,
		"future_page_id":    draft.FuturePageID,
		"reserved_fullpath": draft.ReservedFullpath,
		"updated_at":        draft.UpdatedAt,
		"items_source":      itemsSource,
	}
	return api.PageDocument(base)
}

// warnDraftItemsFallback tells the caller on stderr that the items in the
// document came from the live page, not from the draft snapshot.
func warnDraftItemsFallback(session *surgicalSession) {
	ctx := context.Background()
	if session != nil && session.ctx != nil {
		ctx = session.ctx
	}
	_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err,
		"warning: could not decode draft snapshot; showing live items\n")
}

func draftHeaderLine(ctx context.Context, doc api.PageDocument, draft *api.PageDraft) error {
	fullpath := draftFullpath(draft, api.PageDocumentFullpath(doc))
	_, err := output.Fprintf(ctx, "Draft %s for %s, updated %s\n", draft.ID, fullpath, draft.UpdatedAt)
	return err
}

func printDraftDocumentSummary(ctx context.Context, doc api.PageDocument, draft *api.PageDraft) error {
	if err := draftHeaderLine(ctx, doc, draft); err != nil {
		return err
	}
	stats := api.PageStats(doc)
	lines := []struct {
		label string
		value any
	}{
		{"ID", doc["id"]},
		{"Fullpath", api.PageDocumentFullpath(doc)},
		{"Title", api.PageDocumentTitle(doc)},
		{"Published", api.PageDocumentPublished(doc)},
		{"Editables", stats.EditableCount},
		{"Attachments", stats.AttachmentCount},
	}
	for _, line := range lines {
		if _, err := output.Fprintf(ctx, "%-13s %v\n", line.label+":", line.value); err != nil {
			return err
		}
	}
	return printDraftCanvasCounts(ctx, doc)
}

func printDraftCanvasCounts(ctx context.Context, doc api.PageDocument) error {
	items, ok := doc["items"].(map[string]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(items))
	for name, raw := range items {
		editable, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, isCanvas := editable["repeatables"].([]any); isCanvas {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	for _, name := range names {
		editable, _ := items[name].(map[string]any)
		reps, _ := editable["repeatables"].([]any)
		if _, err := output.Fprintf(ctx, "%-13s %d blocks\n", name+":", len(reps)); err != nil {
			return err
		}
	}
	return nil
}

func printDraftRawSummary(ctx context.Context, session *surgicalSession, draft *api.PageDraft) error {
	fullpath := draftFullpath(draft, session.fullpath)
	if _, err := output.Fprintf(ctx, "Draft %s for %s, updated %s\n", draft.ID, fullpath, draft.UpdatedAt); err != nil {
		return err
	}
	content := draft.ContentMap()
	if title := stringAny(content["title"]); title != "" {
		if _, err := output.Fprintf(ctx, "title: %s\n", title); err != nil {
			return err
		}
	}
	count := 0
	if items, ok := content["page_items"].([]any); ok {
		count = len(items)
	}
	_, err := output.Fprintf(ctx, "%d page_items\n", count)
	return err
}

// PagesDraftSaveCmd posts a page-like body to /pages/{id}/draft.
type PagesDraftSaveCmd struct {
	Page   string `required:"" help:"Page fullpath or id"`
	File   string `required:"" help:"Draft JSON file (use - for stdin)"`
	Locale string `help:"Content locale for localized fields"`
}

// Run executes pages draft save.
func (c *PagesDraftSaveCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "save page draft"); err != nil {
		return err
	}
	warnDraftLocale(ctx, true, c.Locale)
	session, err := openSurgicalPage(ctx, flags, c.Page, c.Locale)
	if err != nil {
		return err
	}
	raw, err := readJSONInput(c.File)
	if err != nil {
		return err
	}
	body := api.PageDocument(raw)
	if err := api.ExpandPageAttachmentPathsWithOptions(body, pageAttachmentFileRefOptions(ctx, session.client, api.PageAttachmentExpansionOptions{})); err != nil {
		return err
	}
	api.NormalizePageDocumentForWrite(body)
	draft, err := session.client.PostPageDraft(ctx, session.pageID, body, api.DraftOptions{ContentLocale: c.Locale})
	if err != nil {
		return draftAPIError(err, session.fullpath)
	}
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, draft)
	}
	_, err = output.Fprintf(ctx, "Saved draft %s of %s (updated %s)\n", draft.ID, session.fullpath, draft.UpdatedAt)
	return err
}

// PagesDraftBatchCmd is an alias for pages batch --draft.
type PagesDraftBatchCmd struct {
	Page   string `required:"" help:"Page fullpath or id"`
	File   string `required:"" help:"JSON operations object or array"`
	Locale string `help:"Content locale for localized fields"`
	Diff   bool   `help:"Show a unified diff of each changed subtree"`
	DryRun bool   `help:"Resolve paths and print the operations without writing"`
}

// Run executes pages draft batch.
func (c *PagesDraftBatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	return (&PagesBatchCmd{
		Page:   c.Page,
		File:   c.File,
		Atomic: true,
		Locale: c.Locale,
		Diff:   c.Diff,
		DryRun: c.DryRun,
		Draft:  true,
	}).Run(ctx, flags)
}

// PagesDraftPublishCmd publishes a draft to the live page.
type PagesDraftPublishCmd struct {
	Page    string `required:"" help:"Page fullpath or id"`
	Confirm bool   `help:"Replace live content when the live page changed after this draft was based on it"`
}

// Run executes pages draft publish.
func (c *PagesDraftPublishCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "publish page draft"); err != nil {
		return err
	}
	session, err := openSurgicalPage(ctx, flags, c.Page, "")
	if err != nil {
		return err
	}
	page, err := session.client.PublishPageDraft(ctx, session.pageID, c.Confirm)
	if err != nil {
		return draftAPIError(err, session.fullpath)
	}
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, page)
	}
	fullpath := api.PageDocumentFullpath(page)
	if fullpath == "" {
		fullpath = session.fullpath
	}
	_, err = output.Fprintf(ctx, "Published draft to %s (page updated %s)\n", fullpath, stringAny(page["updated_at"]))
	return err
}

// PagesDraftDiscardCmd deletes the current page draft.
type PagesDraftDiscardCmd struct {
	Page string `required:"" help:"Page fullpath or id"`
}

// Run executes pages draft discard.
func (c *PagesDraftDiscardCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "discard page draft"); err != nil {
		return err
	}
	if err := requireForce(flags, "draft for "+c.Page); err != nil {
		return err
	}
	session, err := openSurgicalPage(ctx, flags, c.Page, "")
	if err != nil {
		return err
	}
	if err := session.client.DeletePageDraft(ctx, session.pageID); err != nil {
		return draftAPIError(err, session.fullpath)
	}
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, map[string]any{"status": "discarded", "page": session.fullpath})
	}
	_, err = output.Fprintf(ctx, "Discarded draft of %s\n", session.fullpath)
	return err
}

// PagesDraftPreviewURLCmd prints an absolute draft preview URL.
type PagesDraftPreviewURLCmd struct {
	Page string `required:"" help:"Page fullpath or id"`
	Open bool   `help:"Open the preview URL in the browser"`
}

// Run executes pages draft preview-url.
func (c *PagesDraftPreviewURLCmd) Run(ctx context.Context, flags *RootFlags) error {
	session, err := openSurgicalPage(ctx, flags, c.Page, "")
	if err != nil {
		return err
	}
	token, err := session.client.CreatePageDraftPreviewToken(ctx, session.pageID)
	if err != nil {
		return draftAPIError(err, session.fullpath)
	}
	siteHost := lookupSiteSubdomain(ctx, session.client, siteFromFlags(flags))
	absolute := absolutePreviewURL(token.PreviewURL, stringAny(session.doc["public_url"]), siteHostFromAPI(session.client.BaseURL, siteHost))
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, map[string]any{
			"token":       token.Token,
			"preview_url": absolute,
			"expires_in":  "24h",
		})
	}
	if _, err := output.Fprintln(ctx, absolute); err != nil {
		return err
	}
	if c.Open {
		return openDraftPreview(absolute)
	}
	return nil
}

func loadPageDraft(ctx context.Context, flags *RootFlags, page, locale string) (*surgicalSession, *api.PageDraft, error) {
	session, err := openSurgicalPage(ctx, flags, page, locale)
	if err != nil {
		return nil, nil, err
	}
	draft, err := session.client.GetPageDraft(ctx, session.pageID, api.DraftOptions{ContentLocale: locale})
	if err != nil {
		return nil, nil, draftAPIError(err, session.fullpath)
	}
	return session, draft, nil
}

func draftFullpath(draft *api.PageDraft, fallback string) string {
	if draft != nil && strings.TrimSpace(draft.ReservedFullpath) != "" {
		return draft.ReservedFullpath
	}
	return fallback
}

func draftAPIError(err error, page string) error {
	if err == nil {
		return nil
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	if apiErr.IsForbidden() && strings.EqualFold(strings.TrimSpace(apiErr.Message), "Page drafts are not enabled") {
		return newHintedError(apiErr, errorAuthForbidden, ExitAuthz,
			"page drafts are disabled on this Nimbu server (PAGE_DRAFTS_DISABLED is set); publish directly with pages set/update instead")
	}
	if apiErr.IsNotFound() {
		return newDetailedError(
			fmt.Errorf("no draft for page %s; create one with: nimbu pages set --draft --page %s", page, page),
			errorNotFound,
			ExitNotFound,
			nil,
		)
	}
	if apiErr.StatusCode == 409 && apiErr.Code == "draft_base_changed" {
		hint := fmt.Sprintf("re-run with --confirm to replace the live content, or nimbu pages draft discard --page %s --force to drop the draft", page)
		return newHintedError(apiErr, errorConflict, ExitValidation, hint)
	}
	return err
}

func siteFromFlags(flags *RootFlags) string {
	if flags == nil {
		return ""
	}
	return flags.Site
}

func absolutePreviewURL(previewURL, publicURL, siteHost string) string {
	previewURL = strings.TrimSpace(previewURL)
	if strings.HasPrefix(previewURL, "http://") || strings.HasPrefix(previewURL, "https://") {
		return previewURL
	}
	origin := previewOrigin(publicURL, siteHost)
	if previewURL == "" {
		return origin
	}
	if !strings.HasPrefix(previewURL, "/") {
		previewURL = "/" + previewURL
	}
	return strings.TrimRight(origin, "/") + previewURL
}

func previewOrigin(publicURL, siteHost string) string {
	if parsed, err := url.Parse(strings.TrimSpace(publicURL)); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.Scheme + "://" + parsed.Host
	}
	siteHost = strings.TrimSpace(siteHost)
	if siteHost == "" {
		return ""
	}
	if strings.Contains(siteHost, "://") {
		return strings.TrimRight(siteHost, "/")
	}
	if strings.Contains(siteHost, ".") {
		return "https://" + siteHost
	}
	return "https://" + siteHost + ".nimbu.io"
}

var openDraftPreview = openDraftPreviewDefault

func openDraftPreviewDefault(target string) error {
	return openServerBrowserURL(target, nil)
}
