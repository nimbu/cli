package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	inlineFileFallbackName  = "file"
	fileRefDownloadTimeout  = 30 * time.Second
	maxInlineFileBytesLabel = "25 MB"
)

func (n *FileRefNormalizer) downloadClient() *http.Client {
	if n != nil && n.HTTPClient != nil {
		return n.HTTPClient
	}
	return &http.Client{Timeout: fileRefDownloadTimeout}
}

func (n *FileRefNormalizer) downloadInlineFile(ctx context.Context, rawURL string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	resp, err := n.downloadClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("download %s: HTTP %d", rawURL, resp.StatusCode)
	}
	if resp.ContentLength > maxInlineFileBytes {
		return nil, fmt.Errorf("download %s: file is larger than %s", rawURL, maxInlineFileBytesLabel)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxInlineFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	if int64(len(data)) > maxInlineFileBytes {
		return nil, fmt.Errorf("download %s: file is larger than %s", rawURL, maxInlineFileBytesLabel)
	}

	payload := map[string]any{
		"__type":     "File",
		"attachment": base64.StdEncoding.EncodeToString(data),
		"filename":   inlineFilename(rawURL, resp.Header.Get("Content-Disposition")),
	}
	if ct := mediaTypeOnly(resp.Header.Get("Content-Type")); ct != "" {
		payload["content_type"] = ct
	}
	return payload, nil
}

func inlineFilename(rawURL, disposition string) string {
	if name := filenameFromDisposition(disposition); name != "" {
		return name
	}
	u, err := url.Parse(rawURL)
	if err == nil {
		base := path.Base(u.Path)
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	return inlineFileFallbackName
}

func filenameFromDisposition(header string) string {
	if strings.TrimSpace(header) == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(params["filename"])
	if name == "" {
		return ""
	}
	return path.Base(name)
}

func mediaTypeOnly(header string) string {
	if strings.TrimSpace(header) == "" {
		return ""
	}
	media, _, err := mime.ParseMediaType(header)
	if err != nil {
		return strings.TrimSpace(strings.Split(header, ";")[0])
	}
	return media
}
