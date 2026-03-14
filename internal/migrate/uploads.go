package migrate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
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
	tempDir, err := os.MkdirTemp("", "nimbu-upload-copy-*")
	if err != nil {
		return api.Upload{}, err
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	filename := uploadFilename(source)
	localPath := filepath.Join(tempDir, filepath.Base(filename))
	if err := downloadUploadToFile(ctx, fromClient, source.URL, localPath); err != nil {
		return api.Upload{}, err
	}
	body, err := newUploadMultipartBody(localPath, filename)
	if err != nil {
		return api.Upload{}, err
	}
	var created api.Upload
	if err := toClient.Post(ctx, "/uploads", body, &created); err != nil {
		return api.Upload{}, err
	}
	if strings.TrimSpace(created.URL) == "" {
		return api.Upload{}, fmt.Errorf("target upload %q missing url", created.ID)
	}
	return created, nil
}

func downloadUploadToFile(ctx context.Context, client *api.Client, rawURL string, target string) error {
	resp, resolvedURL, err := openDownloadResponse(ctx, client, rawURL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("download %s: HTTP %d", resolvedURL, resp.StatusCode)
	}
	file, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create temp upload file: %w", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("write temp upload file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temp upload file: %w", err)
	}
	return nil
}

func newUploadMultipartBody(path string, filename string) (api.RequestBody, error) {
	info, err := os.Stat(path)
	if err != nil {
		return api.RequestBody{}, fmt.Errorf("stat file: %w", err)
	}

	headerBuf := &strings.Builder{}
	writer := multipart.NewWriter(headerBuf)
	if _, err := writer.CreateFormFile("file", filename); err != nil {
		return api.RequestBody{}, fmt.Errorf("create form file: %w", err)
	}
	headerLen := headerBuf.Len()
	if err := writer.Close(); err != nil {
		return api.RequestBody{}, fmt.Errorf("close multipart writer: %w", err)
	}
	payload := []byte(headerBuf.String())
	header := append([]byte(nil), payload[:headerLen]...)
	footer := append([]byte(nil), payload[headerLen:]...)
	contentType := writer.FormDataContentType()
	contentLength := int64(len(header)) + info.Size() + int64(len(footer))

	buildReader := func() (io.ReadCloser, error) {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		reader := io.MultiReader(strings.NewReader(string(header)), file, strings.NewReader(string(footer)))
		return &uploadMultipartReadCloser{Reader: reader, file: file}, nil
	}

	reader, err := buildReader()
	if err != nil {
		return api.RequestBody{}, err
	}
	return api.RequestBody{
		Reader: reader,
		GetBody: func() (io.ReadCloser, error) {
			return buildReader()
		},
		ContentType:   contentType,
		ContentLength: contentLength,
	}, nil
}

type uploadMultipartReadCloser struct {
	io.Reader
	file *os.File
}

func (r *uploadMultipartReadCloser) Close() error {
	if r.file == nil {
		return nil
	}
	return r.file.Close()
}

func newMultipartBytesBody(data []byte, filename string) (api.RequestBody, error) {
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return api.RequestBody{}, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return api.RequestBody{}, fmt.Errorf("write multipart payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return api.RequestBody{}, fmt.Errorf("close multipart writer: %w", err)
	}

	body := payload.Bytes()
	return api.RequestBody{
		Reader: bytes.NewReader(body),
		GetBody: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		},
		ContentType:   writer.FormDataContentType(),
		ContentLength: int64(len(body)),
	}, nil
}
