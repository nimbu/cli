package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
	Page string `required:"" help:"Page fullpath or id"`
}

// Run executes pages draft get.
func (c *PagesDraftGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	session, draft, err := loadPageDraft(ctx, flags, c.Page, "")
	if err != nil {
		return err
	}
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, draft)
	}
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
	_, err = output.Fprintf(ctx, "%d page_items\n", count)
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
	session, err := openSurgicalPage(ctx, flags, c.Page, c.Locale)
	if err != nil {
		return err
	}
	raw, err := readJSONInput(c.File)
	if err != nil {
		return err
	}
	body := api.PageDocument(raw)
	if err := api.ExpandPageAttachmentPaths(body); err != nil {
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
		return newHintedError(fmt.Errorf("%s; %s", apiErr.Message, hint), errorConflict, ExitValidation, hint)
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
