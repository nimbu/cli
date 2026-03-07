package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
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

// GetPageDocument fetches the full page document by fullpath.
func GetPageDocument(ctx context.Context, c *Client, fullpath string, opts ...RequestOption) (PageDocument, error) {
	var doc PageDocument
	path := "/pages/" + neturl.PathEscape(NormalizePageFullpath(fullpath))
	if err := c.Get(ctx, path, &doc, opts...); err != nil {
		return nil, err
	}
	return doc, nil
}

// PatchPageDocument updates a page document with replace semantics.
func PatchPageDocument(ctx context.Context, c *Client, fullpath string, doc PageDocument, opts ...RequestOption) (PageDocument, error) {
	var out PageDocument
	path := "/pages/" + neturl.PathEscape(NormalizePageFullpath(fullpath))
	opts = append(opts, WithParam("replace", "1"))
	if err := c.Patch(ctx, path, doc, &out, opts...); err != nil {
		return nil, err
	}
	return out, nil
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
	_ = walkPageEditables(doc, func(_ string, editable map[string]any) error {
		stats.EditableCount++
		file := pageEditableFile(editable)
		if file != nil && pageFileHasAttachment(file) {
			stats.AttachmentCount++
		}
		return nil
	})
	return stats
}

// ExpandPageAttachmentPaths loads local file refs from attachment_path and rewrites them for API upload.
func ExpandPageAttachmentPaths(doc PageDocument) error {
	return walkPageEditables(doc, func(_ string, editable map[string]any) error {
		file := pageEditableFile(editable)
		if file == nil {
			return nil
		}

		rawPath := stringValue(file["attachment_path"])
		if rawPath == "" {
			return nil
		}

		data, err := os.ReadFile(rawPath)
		if err != nil {
			return fmt.Errorf("read attachment_path %q: %w", rawPath, err)
		}

		file["attachment"] = base64.StdEncoding.EncodeToString(data)
		if stringValue(file["filename"]) == "" {
			file["filename"] = filepath.Base(rawPath)
		}
		delete(file, "url")
		delete(file, "attachment_path")
		return nil
	})
}

// DownloadPageAssets downloads remote file editables and rewrites them to local attachment_path refs.
func DownloadPageAssets(ctx context.Context, c *Client, doc PageDocument, dir string) (int, error) {
	if strings.TrimSpace(dir) == "" {
		return 0, fmt.Errorf("download directory required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("create download directory: %w", err)
	}

	usedNames := map[string]struct{}{}
	downloads := 0
	err := walkPageEditables(doc, func(_ string, editable map[string]any) error {
		file := pageEditableFile(editable)
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
			return err
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
		return downloads, err
	}

	return downloads, nil
}

func walkPageEditables(doc PageDocument, fn func(name string, editable map[string]any) error) error {
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

func pageEditableFile(editable map[string]any) map[string]any {
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
	case pageFileURL(file) != "":
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
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse asset url %q: %w", rawURL, err)
	}
	if !u.IsAbs() {
		base, baseErr := neturl.Parse(c.BaseURL)
		if baseErr != nil {
			return fmt.Errorf("parse base url: %w", baseErr)
		}
		u = base.ResolveReference(u)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("build asset request: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download asset %q: %w", u.String(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download asset %q: %s", u.String(), strings.TrimSpace(string(body)))
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
