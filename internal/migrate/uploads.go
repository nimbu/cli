package migrate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

type uploadReuseKey struct {
	Name string
	Size int64
}

// UploadCopyItem describes one copied or reused upload.
type UploadCopyItem struct {
	Action    string `json:"action"`
	Name      string `json:"name"`
	Size      int64  `json:"size,omitempty"`
	SourceID  string `json:"source_id,omitempty"`
	SourceURL string `json:"source_url,omitempty"`
	TargetID  string `json:"target_id,omitempty"`
	TargetURL string `json:"target_url,omitempty"`
}

// UploadCopyResult reports upload copy work.
type UploadCopyResult struct {
	From     SiteRef          `json:"from"`
	To       SiteRef          `json:"to"`
	Items    []UploadCopyItem `json:"items,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
}

func CopyUploads(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, dryRun bool) (UploadCopyResult, *MediaRewritePlan, error) {
	sourceUploads, err := api.List[api.Upload](ctx, fromClient, "/uploads")
	if err != nil {
		return UploadCopyResult{From: fromRef, To: toRef}, nil, err
	}
	targetUploads, err := api.List[api.Upload](ctx, toClient, "/uploads")
	if err != nil {
		return UploadCopyResult{From: fromRef, To: toRef}, nil, err
	}
	sourceCounts := map[uploadReuseKey]int{}
	sourceHosts := map[string]struct{}{}
	for _, upload := range sourceUploads {
		sourceCounts[reuseKey(upload)]++
		if host := uploadHost(upload.URL); host != "" {
			sourceHosts[host] = struct{}{}
		}
	}

	targetIndex := make(map[uploadReuseKey][]api.Upload)
	for _, upload := range targetUploads {
		key := reuseKey(upload)
		targetIndex[key] = append(targetIndex[key], upload)
	}

	result := UploadCopyResult{From: fromRef, To: toRef}
	media := NewMediaRewritePlan()
	sourceOrigins := siteOrigins(ctx, fromClient, fromRef.Site)
	for _, origin := range sourceOrigins {
		media.trackSourceURL(origin)
	}
	for i, sourceUpload := range sourceUploads {
		emitStageItem(ctx, "Uploads", sourceUpload.Name, int64(i+1), int64(len(sourceUploads)))
		if strings.TrimSpace(sourceUpload.URL) == "" {
			result.Items = append(result.Items, UploadCopyItem{
				Action:   "skip",
				Name:     sourceUpload.Name,
				Size:     sourceUpload.Size,
				SourceID: sourceUpload.ID,
			})
			result.Warnings = append(result.Warnings, fmt.Sprintf("skip upload %s: source upload missing url", sourceUpload.Name))
			continue
		}

		targetUpload, action, warning, err := resolveTargetUpload(ctx, fromClient, toClient, sourceUpload, targetIndex, sourceCounts, dryRun)
		if err != nil {
			return result, nil, fmt.Errorf("copy upload %s: %w", sourceUpload.Name, err)
		}
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
		targetIndex[reuseKey(targetUpload)] = append(targetIndex[reuseKey(targetUpload)], targetUpload)
		if strings.TrimSpace(targetUpload.URL) != "" {
			media.Add(sourceUpload.URL, targetUpload.URL)
			for _, origin := range sourceOrigins {
				if alias := sourceUploadAlias(origin, sourceUpload.URL); alias != "" {
					media.Add(alias, targetUpload.URL)
				}
			}
		}
		result.Items = append(result.Items, UploadCopyItem{
			Action:    action,
			Name:      sourceUpload.Name,
			Size:      sourceUpload.Size,
			SourceID:  sourceUpload.ID,
			SourceURL: sourceUpload.URL,
			TargetID:  targetUpload.ID,
			TargetURL: targetUpload.URL,
		})
	}
	for host := range sourceHosts {
		media.sourceHosts[host] = struct{}{}
	}
	return result, media, nil
}

func siteOrigins(ctx context.Context, client *api.Client, site string) []string {
	var details api.Site
	if err := client.Get(ctx, "/sites/"+url.PathEscape(strings.TrimSpace(site)), &details); err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	var origins []string
	add := func(raw string) {
		if origin := normalizeOrigin(raw); origin != "" {
			if _, ok := seen[origin]; ok {
				return
			}
			seen[origin] = struct{}{}
			origins = append(origins, origin)
		}
	}
	add(details.Domain)
	if details.Subdomain != "" {
		if apiBase, err := url.Parse(client.BaseURL); err == nil {
			host := strings.ToLower(apiBase.Hostname())
			if strings.HasPrefix(host, "api.") {
				root := strings.TrimPrefix(host, "api.")
				add("https://" + details.Subdomain + "." + root)
			}
		}
	}
	return origins
}

func normalizeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func sourceUploadAlias(origin, sourceURL string) string {
	originURL, err := url.Parse(origin)
	if err != nil || originURL.Host == "" {
		return ""
	}
	sourceParsed, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil {
		return ""
	}
	alias := *originURL
	alias.Path = sourceParsed.Path
	alias.RawPath = sourceParsed.RawPath
	return alias.String()
}

func resolveTargetUpload(ctx context.Context, fromClient, toClient *api.Client, source api.Upload, targetIndex map[uploadReuseKey][]api.Upload, sourceCounts map[uploadReuseKey]int, dryRun bool) (api.Upload, string, string, error) {
	key := reuseKey(source)
	if sourceCounts[key] == 1 {
		switch len(targetIndex[key]) {
		case 1:
			return targetIndex[key][0], "reuse", "", nil
		case 0:
		default:
			if dryRun {
				return api.Upload{Name: source.Name, Size: source.Size}, "dry-run:create", fmt.Sprintf("create upload %s: ambiguous target match by name and size", source.Name), nil
			}
			created, err := createUpload(ctx, fromClient, toClient, source)
			if err != nil {
				return api.Upload{}, "", "", err
			}
			return created, "create", fmt.Sprintf("create upload %s: ambiguous target match by name and size", source.Name), nil
		}
	}

	if dryRun {
		return api.Upload{Name: source.Name, Size: source.Size}, "dry-run:create", "", nil
	}
	created, err := createUpload(ctx, fromClient, toClient, source)
	if err != nil {
		return api.Upload{}, "", "", err
	}
	return created, "create", "", nil
}

func reuseKey(upload api.Upload) uploadReuseKey {
	return uploadReuseKey{
		Name: strings.TrimSpace(upload.Name),
		Size: upload.Size,
	}
}

func uploadFilename(upload api.Upload) string {
	if name := strings.TrimSpace(upload.Name); name != "" {
		return name
	}
	parsed, err := url.Parse(upload.URL)
	if err == nil {
		if base := path.Base(parsed.Path); base != "" && base != "." && base != "/" {
			return base
		}
	}
	return "upload.bin"
}

func uploadHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Host)
}

func createUpload(ctx context.Context, fromClient, toClient *api.Client, source api.Upload) (api.Upload, error) {
	filename := uploadFilename(source)
	data, err := downloadUploadBytes(ctx, fromClient, source.URL, source.Size)
	if err != nil {
		return api.Upload{}, err
	}

	body := api.NewUploadCreatePayload(filename, data, source.MimeType)
	var created api.Upload
	if err := toClient.Post(ctx, "/uploads", body, &created); err != nil {
		return api.Upload{}, err
	}
	if strings.TrimSpace(created.URL) == "" {
		return api.Upload{}, fmt.Errorf("target upload %q missing url", created.ID)
	}
	if strings.TrimSpace(created.Name) == "" {
		return api.Upload{}, fmt.Errorf("target upload %q missing name", created.ID)
	}
	if source.Size > 0 && created.Size != source.Size {
		return api.Upload{}, fmt.Errorf("target upload %q size mismatch: got %d bytes, want %d", created.ID, created.Size, source.Size)
	}
	return created, nil
}

func downloadUploadBytes(ctx context.Context, client *api.Client, rawURL string, expectedSize int64) ([]byte, error) {
	resp, resolvedURL, err := openDownloadResponse(ctx, client, rawURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("download %s: HTTP %d", resolvedURL, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upload bytes: %w", err)
	}
	if expectedSize > 0 && int64(len(data)) != expectedSize {
		return nil, fmt.Errorf("download %s: size mismatch: got %d bytes, want %d", resolvedURL, len(data), expectedSize)
	}
	return data, nil
}
