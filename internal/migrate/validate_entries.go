package migrate

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

// enrichedFetchOpts returns request options that enable rich file metadata
// (checksum, size, dimensions) and reference slugs in API responses.
func enrichedFetchOpts() []api.RequestOption {
	return []api.RequestOption{
		api.WithHeader("X-Nimbu-Client-Version", "nimbu-go-cli"),
		api.WithParam("include_slugs", "true"),
	}
}

// ValidateEntries re-fetches source and target entries after copy, compares
// them field-by-field, and returns warnings for any mismatches. It never
// returns an error — fetch failures are reported as warnings.
func ValidateEntries(ctx context.Context, fromClient, toClient *api.Client,
	mapping map[string]map[string]string,
	channelMap map[string]api.ChannelDetail,
) []string {
	var warnings []string
	channels := sortedKeys(mapping)
	total := len(channels)

	for i, channel := range channels {
		idMap := mapping[channel]
		if len(idMap) == 0 {
			continue
		}

		emitStageItem(ctx, "Validation", channel, int64(i+1), int64(total))

		detail, ok := channelMap[channel]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("channel=%s: schema not found, skipping validation", channel))
			continue
		}
		info := buildSchemaInfo(channel, detail.Customizations)

		sourceIndex, err := fetchEnrichedEntries(ctx, fromClient, channel)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("channel=%s: source fetch failed: %v", channel, err))
			continue
		}

		targetIndex, err := fetchEnrichedEntries(ctx, toClient, channel)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("channel=%s: target fetch failed: %v", channel, err))
			continue
		}

		w := validateChannel(channel, idMap, sourceIndex, targetIndex, info)
		warnings = append(warnings, w...)
	}

	return warnings
}

// fetchEnrichedEntries lists all entries for a channel with enriched metadata
// and indexes them by ID.
func fetchEnrichedEntries(ctx context.Context, client *api.Client, channel string) (map[string]map[string]any, error) {
	path := "/channels/" + url.PathEscape(channel) + "/entries"
	entries, err := api.List[map[string]any](ctx, client, path, enrichedFetchOpts()...)
	if err != nil {
		return nil, err
	}
	index := make(map[string]map[string]any, len(entries))
	for _, entry := range entries {
		if id := stringValue(entry["id"]); id != "" {
			index[id] = entry
		}
	}
	return index, nil
}

// validateChannel compares source→target entry pairs for one channel.
func validateChannel(channel string, idMap map[string]string,
	sourceIndex, targetIndex map[string]map[string]any,
	info schemaInfo,
) []string {
	var warnings []string

	for sourceID, targetID := range idMap {
		source, ok := sourceIndex[sourceID]
		if !ok {
			continue // source entry gone — nothing to compare
		}
		identifier := recordIdentifier(source)

		target, ok := targetIndex[targetID]
		if !ok {
			warnings = append(warnings, fmt.Sprintf(
				"channel=%s entry=%s: missing on target (expected targetID=%s)",
				channel, identifier, targetID))
			continue
		}

		w := validateEntry(channel, identifier, source, target, info)
		warnings = append(warnings, w...)
	}

	return warnings
}

// skipFields are system/internal fields that should not be compared.
var skipFields = map[string]bool{
	"id": true, "_id": true,
	"created_at": true, "updated_at": true,
	"url": true, "entries_url": true,
	"short_id": true,
}

// validateEntry compares two entries field-by-field using schema info.
func validateEntry(channel, identifier string, source, target map[string]any, info schemaInfo) []string {
	var warnings []string
	prefix := fmt.Sprintf("channel=%s entry=%s", channel, identifier)

	fieldTypeMap := buildFieldTypeMap(info)

	// Compare all non-skip fields present in source
	for fieldName, sourceVal := range source {
		if skipFields[fieldName] {
			continue
		}
		targetVal := target[fieldName]
		fieldType := fieldTypeMap[fieldName]

		if w := compareField(prefix, fieldName, fieldType, sourceVal, targetVal); w != "" {
			warnings = append(warnings, w)
		}
	}

	return warnings
}

// buildFieldTypeMap creates a name→type lookup from schemaInfo for dispatch.
func buildFieldTypeMap(info schemaInfo) map[string]string {
	m := map[string]string{}
	for _, f := range info.fileFields {
		m[f.Name] = "file"
	}
	for _, f := range info.galleryFields {
		m[f.Name] = "gallery"
	}
	for _, f := range info.referenceFields {
		m[f.Name] = f.Type // "belongs_to", "belongs_to_many", "customer"
	}
	for _, f := range info.selectFields {
		m[f.Name] = "select"
	}
	for _, f := range info.multiFields {
		m[f.Name] = "multi_select"
	}
	return m
}

// compareField dispatches comparison by field type and returns a warning or "".
func compareField(prefix, fieldName, fieldType string, source, target any) string {
	switch fieldType {
	case "file":
		return compareFileField(prefix, fieldName, source, target)
	case "gallery":
		return compareGalleryField(prefix, fieldName, source, target)
	case "belongs_to", "customer":
		return compareRefField(prefix, fieldName, source, target)
	case "belongs_to_many":
		return compareRelationField(prefix, fieldName, source, target)
	case "select":
		return compareSelectField(prefix, fieldName, source, target)
	case "multi_select":
		return compareMultiSelectField(prefix, fieldName, source, target)
	default:
		return compareScalarField(prefix, fieldName, source, target)
	}
}

// --- File comparison ---

type fileInfo struct {
	Filename string
	Checksum string
}

func extractFileInfo(raw any) fileInfo {
	m, ok := raw.(map[string]any)
	if !ok {
		return fileInfo{}
	}
	return fileInfo{
		Filename: stringValue(m["filename"]),
		Checksum: stringValue(m["checksum"]),
	}
}

func compareFileField(prefix, fieldName string, source, target any) string {
	if source == nil && target == nil {
		return ""
	}
	if source == nil && target != nil {
		return fmt.Sprintf("%s field=%s: unexpected file on target", prefix, fieldName)
	}
	if source != nil && target == nil {
		return fmt.Sprintf("%s field=%s: file missing on target", prefix, fieldName)
	}

	sf := extractFileInfo(source)
	tf := extractFileInfo(target)

	if sf.Filename != tf.Filename {
		return fmt.Sprintf("%s field=%s: filename mismatch (source=%q, target=%q)",
			prefix, fieldName, sf.Filename, tf.Filename)
	}
	if sf.Checksum != "" && tf.Checksum != "" && sf.Checksum != tf.Checksum {
		return fmt.Sprintf("%s field=%s: checksum mismatch (source=%s, target=%s)",
			prefix, fieldName, sf.Checksum, tf.Checksum)
	}
	return ""
}

// --- Gallery comparison ---

type galleryImageInfo struct {
	Position int
	Caption  string
	File     fileInfo
}

func extractGalleryImages(raw any) []galleryImageInfo {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	var images []galleryImageInfo
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		pos, _ := m["position"].(float64)
		img := galleryImageInfo{
			Position: int(pos),
			Caption:  stringValue(m["caption"]),
			File:     extractFileInfo(m["file"]),
		}
		images = append(images, img)
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Position < images[j].Position })
	return images
}

func compareGalleryField(prefix, fieldName string, source, target any) string {
	sourceImages := extractGalleryImages(source)
	targetImages := extractGalleryImages(target)

	if len(sourceImages) != len(targetImages) {
		return fmt.Sprintf("%s field=%s: gallery image count mismatch (source=%d, target=%d)",
			prefix, fieldName, len(sourceImages), len(targetImages))
	}

	for i, si := range sourceImages {
		ti := targetImages[i]
		if si.Caption != ti.Caption {
			return fmt.Sprintf("%s field=%s[%d]: caption mismatch (source=%q, target=%q)",
				prefix, fieldName, si.Position, si.Caption, ti.Caption)
		}
		if si.File.Filename != ti.File.Filename {
			return fmt.Sprintf("%s field=%s[%d]: filename mismatch (source=%q, target=%q)",
				prefix, fieldName, si.Position, si.File.Filename, ti.File.Filename)
		}
		if si.File.Checksum != "" && ti.File.Checksum != "" && si.File.Checksum != ti.File.Checksum {
			return fmt.Sprintf("%s field=%s[%d]: checksum mismatch (source=%s, target=%s)",
				prefix, fieldName, si.Position, si.File.Checksum, ti.File.Checksum)
		}
	}
	return ""
}

// --- Reference comparison ---

func extractRefSlug(raw any) string {
	m, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	return stringValue(m["slug"])
}

func compareRefField(prefix, fieldName string, source, target any) string {
	if source == nil && target == nil {
		return ""
	}
	if source == nil && target != nil {
		return fmt.Sprintf("%s field=%s: unexpected reference on target", prefix, fieldName)
	}
	if source != nil && target == nil {
		return fmt.Sprintf("%s field=%s: reference missing on target", prefix, fieldName)
	}

	ss := extractRefSlug(source)
	ts := extractRefSlug(target)
	if ss == "" || ts == "" {
		return "" // can't compare without slugs
	}
	if ss != ts {
		return fmt.Sprintf("%s field=%s: slug mismatch (source=%s, target=%s)",
			prefix, fieldName, ss, ts)
	}
	return ""
}

// --- Relation (belongs_to_many) comparison ---

func extractRelationSlugs(raw any) []string {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	objects, ok := m["objects"].([]any)
	if !ok {
		return nil
	}
	var slugs []string
	for _, obj := range objects {
		if slug := extractRefSlug(obj); slug != "" {
			slugs = append(slugs, slug)
		}
	}
	sort.Strings(slugs)
	return slugs
}

func compareRelationField(prefix, fieldName string, source, target any) string {
	ss := extractRelationSlugs(source)
	ts := extractRelationSlugs(target)

	if !reflect.DeepEqual(ss, ts) {
		return fmt.Sprintf("%s field=%s: relation slugs mismatch (source=%v, target=%v)",
			prefix, fieldName, ss, ts)
	}
	return ""
}

// --- Select comparison ---

func normalizeSelectValue(raw any) any {
	m, ok := raw.(map[string]any)
	if !ok {
		return raw
	}
	if v, ok := m["value"]; ok {
		return v
	}
	return raw
}

func compareSelectField(prefix, fieldName string, source, target any) string {
	sv := normalizeSelectValue(source)
	tv := normalizeSelectValue(target)
	if !reflect.DeepEqual(sv, tv) {
		return fmt.Sprintf("%s field=%s: select value mismatch (source=%v, target=%v)",
			prefix, fieldName, sv, tv)
	}
	return ""
}

// --- Multi-select comparison ---

func normalizeMultiSelectValues(raw any) []string {
	switch v := raw.(type) {
	case map[string]any:
		if vals, ok := v["values"].([]any); ok {
			return toSortedStrings(vals)
		}
		return nil
	case []any:
		return toSortedStrings(v)
	default:
		return nil
	}
}

func toSortedStrings(arr []any) []string {
	var out []string
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func compareMultiSelectField(prefix, fieldName string, source, target any) string {
	sv := normalizeMultiSelectValues(source)
	tv := normalizeMultiSelectValues(target)
	if !reflect.DeepEqual(sv, tv) {
		return fmt.Sprintf("%s field=%s: multi-select mismatch (source=%v, target=%v)",
			prefix, fieldName, sv, tv)
	}
	return ""
}

// --- Scalar comparison ---

func compareScalarField(prefix, fieldName string, source, target any) string {
	if reflect.DeepEqual(source, target) {
		return ""
	}
	// Normalize both to strings for common text fields
	if ss, ok := source.(string); ok {
		if ts, ok := target.(string); ok {
			if strings.TrimSpace(ss) == strings.TrimSpace(ts) {
				return ""
			}
		}
	}
	return fmt.Sprintf("%s field=%s: value mismatch", prefix, fieldName)
}

// --- Helpers ---

func sortedKeys(m map[string]map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
