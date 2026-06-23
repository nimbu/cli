package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	neturl "net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/nimbu/cli/internal/output"
)

// PageDocument is the canonical page contract used for get/update flows.
type PageDocument map[string]any

// PageDocumentStats summarizes nested editables inside a page document.
type PageDocumentStats struct {
	AttachmentCount int
	EditableCount   int
}

// PageAttachmentExpansionOptions configures file editable expansion for writes.
type PageAttachmentExpansionOptions struct {
	AllowEmptyFile      bool
	DropEmptyFile       bool
	DropReadOnlyFileURL bool
}

// GetPageDocument fetches the full page document by fullpath.
func GetPageDocument(ctx context.Context, c *Client, fullpath string, opts ...RequestOption) (PageDocument, error) {
	var doc PageDocument
	path := "/pages/" + neturl.PathEscape(NormalizePageFullpath(fullpath))
	if err := c.Get(ctx, path, &doc, opts...); err != nil {
		return nil, err
	}
	return doc, nil
}

// PatchPageDocument updates a page document. The caller controls merge vs.
// replace semantics via opts (see WithReplace); by default the API merges.
func PatchPageDocument(ctx context.Context, c *Client, fullpath string, doc PageDocument, opts ...RequestOption) (PageDocument, error) {
	var out PageDocument
	path := "/pages/" + neturl.PathEscape(NormalizePageFullpath(fullpath))
	if err := c.Patch(ctx, path, doc, &out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// pageReadOnlyKeys are server-managed top-level keys that must never be written back.
var pageReadOnlyKeys = []string{
	"id",
	"created_at",
	"updated_at",
	"creator_id",
	"updater_id",
	"parent_path",
}

// NormalizePageDocumentForWrite removes server-managed top-level keys so the
// document can be safely sent on create/update requests.
func NormalizePageDocumentForWrite(doc PageDocument) {
	for _, key := range pageReadOnlyKeys {
		delete(doc, key)
	}
}

// PageCanvasRepeatableCounts returns, for each canvas editable path, the number
// of repeatables it contains. Nested paths use dot notation and aggregate
// repeatables across repeated instances of the same editable path.
func PageCanvasRepeatableCounts(doc PageDocument) map[string]int {
	counts := map[string]int{}
	items, ok := mapValue(doc["items"])
	if !ok {
		return counts
	}
	pageCanvasRepeatableCounts(items, "", counts)
	return counts
}

func pageCanvasRepeatableCounts(items map[string]any, prefix string, counts map[string]int) {
	for name, rawEditable := range items {
		editable, ok := mapValue(rawEditable)
		if !ok {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		repeatables, ok := sliceValue(editable["repeatables"])
		if !ok {
			continue
		}
		counts[path] += len(repeatables)
		for _, rawRepeatable := range repeatables {
			repeatable, ok := mapValue(rawRepeatable)
			if !ok {
				continue
			}
			childItems, ok := mapValue(repeatable["items"])
			if !ok {
				continue
			}
			pageCanvasRepeatableCounts(childItems, path, counts)
		}
	}
}

// PageCanvasRepeatableInstanceCounts returns repeatable counts for each canvas
// instance. Nested paths include repeatable indexes, e.g. blocks[0].gallery.
func PageCanvasRepeatableInstanceCounts(doc PageDocument) map[string]int {
	counts := map[string]int{}
	items, ok := mapValue(doc["items"])
	if !ok {
		return counts
	}
	pageCanvasRepeatableInstanceCounts(items, "", counts)
	return counts
}

func pageCanvasRepeatableInstanceCounts(items map[string]any, prefix string, counts map[string]int) {
	for name, rawEditable := range items {
		editable, ok := mapValue(rawEditable)
		if !ok {
			continue
		}
		currentPath := name
		if prefix != "" {
			currentPath = prefix + "." + name
		}
		repeatables, ok := sliceValue(editable["repeatables"])
		if !ok {
			continue
		}
		counts[currentPath] = len(repeatables)
		for index, rawRepeatable := range repeatables {
			repeatable, ok := mapValue(rawRepeatable)
			if !ok {
				continue
			}
			childItems, ok := mapValue(repeatable["items"])
			if !ok {
				continue
			}
			pageCanvasRepeatableInstanceCounts(childItems, fmt.Sprintf("%s[%d]", currentPath, index), counts)
		}
	}
}

// PageShape returns a skeleton of the page's editables: editable name -> type,
// and for canvases the list of repeatables with their slug and nested skeleton.
func PageShape(doc PageDocument) any {
	items, ok := mapValue(doc["items"])
	if !ok {
		return map[string]any{}
	}
	return pageShapeItems(items)
}

func pageShapeItems(items map[string]any) map[string]any {
	shape := map[string]any{}
	for name, rawEditable := range items {
		editable, ok := mapValue(rawEditable)
		if !ok {
			continue
		}
		entry := map[string]any{
			"type": stringValue(editable["type"]),
		}
		if repeatables, ok := sliceValue(editable["repeatables"]); ok {
			reps := make([]any, 0, len(repeatables))
			for _, rawRepeatable := range repeatables {
				repeatable, ok := mapValue(rawRepeatable)
				if !ok {
					continue
				}
				rep := map[string]any{
					"slug": stringValue(repeatable["slug"]),
				}
				if childItems, ok := mapValue(repeatable["items"]); ok {
					rep["items"] = pageShapeItems(childItems)
				} else {
					rep["items"] = map[string]any{}
				}
				reps = append(reps, rep)
			}
			entry["repeatables"] = reps
		}
		shape[name] = entry
	}
	return shape
}

// NormalizePageFullpath strips leading slashes and whitespace from a page fullpath.
func NormalizePageFullpath(fullpath string) string {
	fullpath = strings.TrimSpace(fullpath)
	return strings.TrimLeft(fullpath, "/")
}

// PageDocumentFullpath returns the canonical page fullpath when present.
func PageDocumentFullpath(doc PageDocument) string {
	if value := stringValue(doc["fullpath"]); value != "" {
		return NormalizePageFullpath(value)
	}
	return NormalizePageFullpath(stringValue(doc["slug"]))
}

// PageDocumentParentPath returns the parent path for a page when present.
func PageDocumentParentPath(doc PageDocument) string {
	if value := stringValue(doc["parent_path"]); value != "" {
		return NormalizePageFullpath(value)
	}
	if value := stringValue(doc["parent"]); value != "" {
		return NormalizePageFullpath(value)
	}
	return ""
}

// PageDocumentTitle returns the page title when present.
func PageDocumentTitle(doc PageDocument) string {
	return stringValue(doc["title"])
}

// PageDocumentTemplate returns the page template when present.
func PageDocumentTemplate(doc PageDocument) string {
	return stringValue(doc["template"])
}

// PageDocumentLocale returns the page locale when present.
func PageDocumentLocale(doc PageDocument) string {
	return stringValue(doc["locale"])
}

// PageDocumentPublished returns whether the page is published.
func PageDocumentPublished(doc PageDocument) bool {
	return boolValue(doc["published"])
}

// PageStats traverses the page document and counts editables and attachment-bearing file objects.
func PageStats(doc PageDocument) PageDocumentStats {
	stats := PageDocumentStats{}
	_ = WalkPageEditables(doc, func(_ string, editable map[string]any) error {
		stats.EditableCount++
		file := PageEditableFile(editable)
		if file != nil && pageFileHasAttachment(file) {
			stats.AttachmentCount++
		}
		return nil
	})
	return stats
}

// ExpandPageAttachmentPaths prepares file editables for an API write:
//   - attachment_path: read the local file, base64-encode it, mark __type "File"
//   - attachment_url:   rewrite the file to a {__type:FileRef, source:<url>} ref
//
// A file editable that ends up with neither an inline attachment nor a writable
// source is an error, since writing it would silently clear the asset.
func ExpandPageAttachmentPaths(doc PageDocument) error {
	return ExpandPageAttachmentPathsWithOptions(doc, PageAttachmentExpansionOptions{})
}

// ExpandPageAttachmentPathsWithOptions prepares file editables for an API write
// with explicit handling for dangerous empty-file payloads.
func ExpandPageAttachmentPathsWithOptions(doc PageDocument, opts PageAttachmentExpansionOptions) error {
	return WalkPageEditables(doc, func(name string, editable map[string]any) error {
		file := PageEditableFile(editable)
		if file == nil {
			return nil
		}

		rawPath := stringValue(file["attachment_path"])
		if rawPath != "" {
			data, err := os.ReadFile(rawPath)
			if err != nil {
				return fmt.Errorf("read attachment_path %q: %w", rawPath, err)
			}

			file["__type"] = "File"
			file["attachment"] = base64.StdEncoding.EncodeToString(data)
			if stringValue(file["filename"]) == "" {
				file["filename"] = filepath.Base(rawPath)
			}
			stripReadOnlyFileWriteKeys(file)
			return nil
		}

		if stringValue(file["attachment"]) != "" {
			file["__type"] = "File"
			stripReadOnlyFileWriteKeys(file)
			return nil
		}

		if attachmentURL := stringValue(file["attachment_url"]); attachmentURL != "" {
			editable["file"] = map[string]any{
				"__type": "FileRef",
				"source": attachmentURL,
			}
			return nil
		}

		if stringValue(file["source"]) != "" {
			file["__type"] = "FileRef"
			stripReadOnlyFileWriteKeys(file)
			return nil
		}

		if opts.DropReadOnlyFileURL && pageFileURL(file) != "" && !pageFileHasWritePayload(file) {
			delete(editable, "file")
			return nil
		}

		if opts.DropEmptyFile && !pageFileHasWritePayload(file) && pageFileURL(file) == "" {
			delete(editable, "file")
			return nil
		}

		if !opts.AllowEmptyFile && !pageFileHasWritePayload(file) {
			return fmt.Errorf("file editable %q has no attachment, attachment_path, attachment_url, or source; refusing to write an empty file", name)
		}
		return nil
	})
}

func stripReadOnlyFileWriteKeys(file map[string]any) {
	delete(file, "url")
	delete(file, "public_url")
	delete(file, "permanent_url")
	delete(file, "attachment_url")
	delete(file, "attachment_path")
}

// DownloadPageAssets downloads remote file editables and rewrites them to local attachment_path refs.
// Returns the count of successful downloads, any per-asset warnings (non-fatal), and a fatal error.
func DownloadPageAssets(ctx context.Context, c *Client, doc PageDocument, dir string) (int, []string, error) {
	if strings.TrimSpace(dir) == "" {
		return 0, nil, fmt.Errorf("download directory required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, nil, fmt.Errorf("create download directory: %w", err)
	}

	usedNames := map[string]struct{}{}
	downloads := 0
	var warnings []string
	err := WalkPageEditables(doc, func(name string, editable map[string]any) error {
		file := PageEditableFile(editable)
		if file == nil {
			return nil
		}

		rawURL := pageFileURL(file)
		if rawURL == "" {
			return nil
		}

		filename := pageFileFilename(file, rawURL)
		target := uniqueAttachmentPath(dir, filename, usedNames)

		if err := downloadURLToFile(ctx, c, rawURL, target); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
			return nil // continue with next editable
		}

		file["attachment_path"] = target
		if stringValue(file["filename"]) == "" {
			file["filename"] = filepath.Base(target)
		}
		delete(file, "url")
		downloads++
		return nil
	})
	if err != nil {
		return downloads, warnings, err
	}

	return downloads, warnings, nil
}

// WalkPageEditables traverses all editables in a page document, including nested repeatables.
func WalkPageEditables(doc PageDocument, fn func(name string, editable map[string]any) error) error {
	items, ok := mapValue(doc["items"])
	if !ok {
		return nil
	}
	return walkPageEditableItems(items, fn)
}

func walkPageEditableItems(items map[string]any, fn func(name string, editable map[string]any) error) error {
	for name, rawEditable := range items {
		editable, ok := mapValue(rawEditable)
		if !ok {
			continue
		}
		if err := fn(name, editable); err != nil {
			return err
		}

		repeatables, ok := sliceValue(editable["repeatables"])
		if !ok {
			continue
		}
		for _, rawRepeatable := range repeatables {
			repeatable, ok := mapValue(rawRepeatable)
			if !ok {
				continue
			}
			childItems, ok := mapValue(repeatable["items"])
			if !ok {
				continue
			}
			if err := walkPageEditableItems(childItems, fn); err != nil {
				return err
			}
		}
	}
	return nil
}

// PageEditableFile returns the file map from an editable, or nil if absent.
func PageEditableFile(editable map[string]any) map[string]any {
	file, ok := mapValue(editable["file"])
	if !ok {
		return nil
	}
	return file
}

func pageFileHasAttachment(file map[string]any) bool {
	switch {
	case stringValue(file["attachment"]) != "":
		return true
	case stringValue(file["attachment_path"]) != "":
		return true
	case stringValue(file["attachment_url"]) != "":
		return true
	case stringValue(file["source"]) != "":
		return true
	case pageFileURL(file) != "":
		return true
	default:
		return false
	}
}

func pageFileHasWritePayload(file map[string]any) bool {
	switch {
	case stringValue(file["attachment"]) != "":
		return true
	case stringValue(file["attachment_path"]) != "":
		return true
	case stringValue(file["attachment_url"]) != "":
		return true
	case stringValue(file["source"]) != "":
		return true
	default:
		return false
	}
}

func pageFileURL(file map[string]any) string {
	for _, key := range []string{"url", "public_url", "permanent_url"} {
		if value := stringValue(file[key]); value != "" {
			return value
		}
	}
	return ""
}

func pageFileFilename(file map[string]any, rawURL string) string {
	if filename := stringValue(file["filename"]); filename != "" {
		return filepath.Base(filename)
	}

	parsed, err := neturl.Parse(rawURL)
	if err == nil {
		base := path.Base(parsed.Path)
		if base != "" && base != "." && base != "/" {
			return filepath.Base(base)
		}
	}

	return "attachment.bin"
}

func uniqueAttachmentPath(dir, filename string, used map[string]struct{}) string {
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		filename = "attachment.bin"
	}

	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)
	candidate := filename
	for index := 2; ; index++ {
		if _, exists := used[candidate]; !exists {
			used[candidate] = struct{}{}
			return filepath.Join(dir, candidate)
		}
		candidate = fmt.Sprintf("%s-%d%s", name, index, ext)
	}
}

func downloadURLToFile(ctx context.Context, c *Client, rawURL, target string) error {
	resp, resolvedURL, err := c.DownloadURL(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("download asset %q: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("download asset %q: HTTP %d", resolvedURL, resp.StatusCode)
	}

	temp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create asset file %q: %w", target, err)
	}
	tempName := temp.Name()
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(tempName)
		}
	}()

	task := output.ProgressFromContext(ctx).Transfer("download "+filepath.Base(target), resp.ContentLength)
	if _, err := io.Copy(temp, task.WrapReadCloser(resp.Body)); err != nil {
		task.Fail(err)
		return fmt.Errorf("write asset file %q: %w", target, err)
	}
	if err := temp.Close(); err != nil {
		task.Fail(err)
		return fmt.Errorf("close asset file %q: %w", target, err)
	}
	if err := os.Rename(tempName, target); err != nil {
		task.Fail(err)
		return fmt.Errorf("finalize asset file %q: %w", target, err)
	}
	removeTemp = false
	task.Done("done")

	return nil
}

func mapValue(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func sliceValue(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func stringValue(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func boolValue(v any) bool {
	b, ok := v.(bool)
	return ok && b
}
