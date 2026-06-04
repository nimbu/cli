package cmd

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

type galleryImageRow struct {
	ID       string `json:"id,omitempty"`
	Position int    `json:"position"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
	URL      string `json:"url,omitempty"`
	Size     int64  `json:"size,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

func galleryPayload(field string, images []map[string]any) map[string]any {
	return map[string]any{
		field: map[string]any{
			"__type": "Gallery",
			"images": images,
		},
	}
}

func buildNewGalleryImages(paths, captions []string, positions []int, startPosition int) ([]map[string]any, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one --image is required")
	}
	if len(captions) > 0 && len(captions) != len(paths) {
		return nil, fmt.Errorf("--caption count must match --image count")
	}
	if len(positions) > 0 && len(positions) != len(paths) {
		return nil, fmt.Errorf("--position count must match --image count")
	}
	images := make([]map[string]any, 0, len(paths))
	for idx, path := range paths {
		file, err := galleryFilePayload(path)
		if err != nil {
			return nil, err
		}
		position := startPosition + idx
		if len(positions) > 0 {
			position = positions[idx]
		}
		image := map[string]any{
			"__type":   "GalleryImage",
			"position": position,
			"file":     file,
		}
		if len(captions) > 0 {
			image["caption"] = captions[idx]
		}
		images = append(images, image)
	}
	return images, nil
}

func buildGalleryImagePatch(id string, caption *string, position *int, imagePath string) (map[string]any, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("--image-id is required")
	}
	image := map[string]any{
		"__type": "GalleryImage",
		"id":     id,
	}
	if caption != nil {
		image["caption"] = *caption
	}
	if position != nil {
		image["position"] = *position
	}
	if imagePath != "" {
		file, err := galleryFilePayload(imagePath)
		if err != nil {
			return nil, err
		}
		image["file"] = file
	}
	if len(image) == 2 {
		return nil, fmt.Errorf("provide at least one of --caption, --position, or --image")
	}
	return image, nil
}

func buildGalleryRemovePatches(ids []string) ([]map[string]any, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one --image-id is required")
	}
	images := make([]map[string]any, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("--image-id cannot be empty")
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate image id %q", id)
		}
		seen[id] = struct{}{}
		images = append(images, map[string]any{
			"__type": "GalleryImage",
			"id":     id,
			"remove": true,
		})
	}
	return images, nil
}

func galleryImageIDsFromPatches(images []map[string]any) []string {
	ids := make([]string, 0, len(images))
	for _, image := range images {
		id := stringFromAny(image["id"])
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func validateGalleryImageIDs(rows []galleryImageRow, ids []string) error {
	existing := map[string]struct{}{}
	for _, row := range rows {
		if row.ID != "" {
			existing[row.ID] = struct{}{}
		}
	}
	for _, id := range ids {
		if _, ok := existing[id]; !ok {
			return fmt.Errorf("unknown image id %q", id)
		}
	}
	return nil
}

func buildGalleryReorderPatches(rows []galleryImageRow, rawOrder string) ([]map[string]any, error) {
	order := splitGalleryOrder(rawOrder)
	if len(order) == 0 {
		return nil, fmt.Errorf("--order must include at least one image id")
	}
	existing := map[string]struct{}{}
	for _, row := range rows {
		if row.ID != "" {
			existing[row.ID] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	images := make([]map[string]any, 0, len(order))
	for idx, id := range order {
		if _, ok := existing[id]; !ok {
			return nil, fmt.Errorf("unknown image id %q", id)
		}
		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("duplicate image id %q", id)
		}
		seen[id] = struct{}{}
		images = append(images, map[string]any{
			"__type":   "GalleryImage",
			"id":       id,
			"position": idx,
		})
	}
	for id := range existing {
		if _, ok := seen[id]; !ok {
			return nil, fmt.Errorf("missing image id %q", id)
		}
	}
	return images, nil
}

func splitGalleryOrder(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func galleryFilePayload(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read image %q: %w", path, err)
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	file := map[string]any{
		"__type":     "File",
		"attachment": base64.StdEncoding.EncodeToString(data),
		"filename":   filepath.Base(path),
	}
	if contentType != "" {
		file["content_type"] = contentType
	}
	return file, nil
}

func galleryRowsFromEntry(entry api.Entry, field string) []galleryImageRow {
	raw, ok := entry.Extra[field]
	if !ok && entry.Fields != nil {
		raw = entry.Fields[field]
	}
	rawImages := galleryRawImages(raw)
	rows := make([]galleryImageRow, 0, len(rawImages))
	for idx, rawImage := range rawImages {
		image, ok := rawImage.(map[string]any)
		if !ok {
			continue
		}
		row := galleryImageRow{
			ID:       stringFromAny(image["id"]),
			Position: intFromAny(image["position"], idx),
			Caption:  stringFromAny(image["caption"]),
		}
		if file, ok := image["file"].(map[string]any); ok {
			row.Filename = firstNonEmptyString(file["filename"], file["name"])
			row.URL = firstNonEmptyString(file["url"], file["public_url"])
			row.Size = int64FromAny(file["size"])
			row.MimeType = firstNonEmptyString(file["content_type"], file["mime_type"])
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].Position < rows[j].Position
	})
	return rows
}

func galleryRawImages(raw any) []any {
	switch typed := raw.(type) {
	case []any:
		return typed
	case map[string]any:
		if images, ok := typed["images"].([]any); ok {
			return images
		}
	}
	return nil
}

func nextGalleryPosition(rows []galleryImageRow) int {
	next := 0
	for _, row := range rows {
		if row.Position >= next {
			next = row.Position + 1
		}
	}
	return next
}

func stringFromAny(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s := stringFromAny(value); strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func intFromAny(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	}
	return fallback
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	}
	return 0
}
