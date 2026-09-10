package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

const (
	nimbuCDNHost             = "cdn.nimbu.io"
	maxFileRefUploadPages    = 20
	fileRefUploadPageSize    = 100
	fileRefCopyWarningSuffix = "is not an upload of this site; the server will copy it"
)

// FileRefNormalizer turns remote file URLs into write payloads.
// Same-site CDN URLs become nimbu:// upload references; everything else is
// passed through as {"__type":"FileRef","source":"<url>"} so the server copies the asset.
type FileRefNormalizer struct {
	Client      *Client
	SiteShortID string
}

// NormalizeURL rewrites a remote file URL into a FileRef write payload.
func (n *FileRefNormalizer) NormalizeURL(ctx context.Context, rawURL string) (map[string]any, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, "", fmt.Errorf("empty file URL")
	}
	if strings.HasPrefix(rawURL, "nimbu://") {
		return nimbuFileRef(rawURL), "", nil
	}

	cdnSite, isCDN := ParseNimbuCDNSiteShortID(rawURL)
	if !isCDN {
		return copyFileRef(rawURL), "", nil
	}

	siteShort, err := n.EnsureSiteShortID(ctx)
	if err == nil && siteShort != "" && !strings.EqualFold(cdnSite, siteShort) {
		return copyFileRef(rawURL), FileRefCopyWarning(rawURL), nil
	}

	upload, err := n.findUploadByURL(ctx, rawURL)
	if err != nil || upload == nil || strings.TrimSpace(upload.ID) == "" {
		return copyFileRef(rawURL), FileRefCopyWarning(rawURL), nil
	}
	if siteShort == "" {
		siteShort = cdnSite
	}
	return nimbuFileRef("nimbu://" + siteShort + "/uploads/" + upload.ID), "", nil
}

// EnsureSiteShortID returns the cached short id or resolves it from the API.
func (n *FileRefNormalizer) EnsureSiteShortID(ctx context.Context) (string, error) {
	if n == nil {
		return "", fmt.Errorf("file ref normalizer is nil")
	}
	if id := strings.TrimSpace(n.SiteShortID); id != "" {
		return id, nil
	}
	if n.Client == nil {
		return "", fmt.Errorf("no API client")
	}
	id, err := ResolveSiteShortID(ctx, n.Client)
	if err != nil {
		return "", err
	}
	n.SiteShortID = id
	return id, nil
}

// ResolveSiteShortID reads site_short_id from GET /themes or GET /themes/:id/info,
// falling back to the /s/<id>/ segment of cdn_root.
func ResolveSiteShortID(ctx context.Context, client *Client) (string, error) {
	if client == nil {
		return "", fmt.Errorf("no API client")
	}
	paged, err := ListPage[Theme](ctx, client, "/themes", 1, 25)
	if err != nil {
		return "", fmt.Errorf("list themes: %w", err)
	}
	for _, theme := range paged.Data {
		if id := strings.TrimSpace(theme.SiteShortID); id != "" {
			return id, nil
		}
		if id, ok := ParseNimbuCDNSiteShortID(theme.CDNRoot); ok {
			return id, nil
		}
	}
	if len(paged.Data) == 0 || strings.TrimSpace(paged.Data[0].ID) == "" {
		return "", fmt.Errorf("no themes to resolve site short id")
	}
	var info struct {
		SiteShortID string `json:"site_short_id"`
		CDNRoot     string `json:"cdn_root"`
	}
	path := "/themes/" + url.PathEscape(paged.Data[0].ID) + "/info"
	if err := client.Get(ctx, path, &info); err != nil {
		return "", fmt.Errorf("get theme info: %w", err)
	}
	if id := strings.TrimSpace(info.SiteShortID); id != "" {
		return id, nil
	}
	if id, ok := ParseNimbuCDNSiteShortID(info.CDNRoot); ok {
		return id, nil
	}
	return "", fmt.Errorf("theme info did not include site_short_id")
}

// ParseNimbuCDNSiteShortID returns the /s/<id>/ segment of a cdn.nimbu.io URL.
func ParseNimbuCDNSiteShortID(rawURL string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Hostname() == "" {
		return "", false
	}
	if !strings.EqualFold(u.Hostname(), nimbuCDNHost) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "s" || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// FileRefCopyWarning is the stderr message body for a passthrough CDN URL.
func FileRefCopyWarning(rawURL string) string {
	return rawURL + " " + fileRefCopyWarningSuffix
}

func (n *FileRefNormalizer) findUploadByURL(ctx context.Context, rawURL string) (*Upload, error) {
	if n == nil || n.Client == nil {
		return nil, fmt.Errorf("no API client")
	}
	want := canonicalFileURL(rawURL)
	collected := 0
	for page := 1; page <= maxFileRefUploadPages; page++ {
		paged, err := ListPage[Upload](ctx, n.Client, "/uploads", page, fileRefUploadPageSize)
		if err != nil {
			return nil, err
		}
		for i := range paged.Data {
			if uploadURLMatches(paged.Data[i].URL, rawURL, want) {
				return &paged.Data[i], nil
			}
		}
		collected += len(paged.Data)
		if !paged.HasMore(fileRefUploadPageSize, collected) {
			return nil, nil
		}
	}
	return nil, nil
}

func uploadURLMatches(got, raw, want string) bool {
	if got == raw {
		return true
	}
	return want != "" && canonicalFileURL(got) == want
}

func canonicalFileURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return strings.TrimSpace(raw)
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	u.Scheme = strings.ToLower(u.Scheme)
	return strings.TrimRight(u.String(), "/")
}

func expandRemoteFileRef(opts PageAttachmentExpansionOptions, rawURL string) (map[string]any, error) {
	if opts.FileRef == nil {
		return nimbuFileRef(rawURL), nil
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	payload, warning, err := opts.FileRef.NormalizeURL(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if warning != "" && opts.Warn != nil {
		opts.Warn(warning)
	}
	return payload, nil
}

func nimbuFileRef(source string) map[string]any {
	return map[string]any{"__type": "FileRef", "source": source}
}

func copyFileRef(rawURL string) map[string]any {
	return nimbuFileRef(rawURL)
}
