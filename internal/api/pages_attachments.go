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

// PageAttachmentExpansionOptions configures file editable expansion for writes.
type PageAttachmentExpansionOptions struct {
	AllowEmptyFile      bool
	DropEmptyFile       bool
	DropReadOnlyFileURL bool
	Context             context.Context
	FileRef             *FileRefNormalizer
	Warn                func(string)
}

// ExpandPageAttachmentPaths prepares file editables for an API write:
//   - attachment_path: read the local file, base64-encode it, mark __type "File"
//   - attachment_url:  same-site CDN uploads become nimbu:// FileRefs; other URLs are downloaded and inlined
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

		if stringValue(file["__type"]) == "FileRef" && stringValue(file["source"]) != "" {
			stripReadOnlyFileWriteKeys(file)
			return nil
		}

		if attachmentURL := stringValue(file["attachment_url"]); attachmentURL != "" {
			payload, err := expandRemoteFileRef(opts, attachmentURL)
			if err != nil {
				return err
			}
			editable["file"] = payload
			return nil
		}

		if source := stringValue(file["source"]); source != "" {
			if opts.FileRef != nil && !strings.HasPrefix(source, "nimbu://") {
				payload, err := expandRemoteFileRef(opts, source)
				if err != nil {
					return err
				}
				editable["file"] = payload
				return nil
			}
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
			return nil
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
