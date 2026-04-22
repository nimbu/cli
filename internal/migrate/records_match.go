package migrate

import (
	"context"
	"net/url"

	"github.com/nimbu/cli/internal/api"
)

// preMatchEntries fetches existing target entries and matches them against source
// records by slug (+ title_field_value fallback). Content-identical matches are
// added to the mapping and emitted as "skip" items. Content-different matches are
// stored in preMatched for fast lookup during upsert.
func (c *recordCopier) preMatchEntries(ctx context.Context, sourceChannel, targetChannel string, sourceRecords []map[string]any, localizedSourceRecords map[string]map[string]map[string]any, info schemaInfo) {
	if targetChannel == "customers" {
		return // customers use email-based matching via findExistingID
	}
	path := "/channels/" + url.PathEscape(targetChannel) + "/entries"
	targetRecords, err := api.List[map[string]any](ctx, c.toClient, path)
	if err != nil {
		return // non-fatal: fall back to per-record matching
	}
	if len(targetRecords) == 0 {
		return
	}
	localizedTargetRecords := c.listTargetLocalizedRecords(ctx, targetChannel, info)

	// Build target indexes: slug → entry, title_field_value → entry
	bySlug := make(map[string]map[string]any, len(targetRecords))
	byTitle := make(map[string]map[string]any, len(targetRecords))
	for _, entry := range targetRecords {
		if slug := stringValue(entry["slug"]); slug != "" {
			bySlug[slug] = entry
		}
		if tfv := stringValue(entry["title_field_value"]); tfv != "" {
			byTitle[tfv] = entry
		}
	}

	mapped := c.ensureMapping(targetChannel)
	if c.preMatched == nil {
		c.preMatched = map[string]map[string]string{}
	}
	if c.preMatched[targetChannel] == nil {
		c.preMatched[targetChannel] = map[string]string{}
	}
	if c.preMatchedLocalizedOnly == nil {
		c.preMatchedLocalizedOnly = map[string]map[string]string{}
	}
	if c.preMatchedLocalizedOnly[targetChannel] == nil {
		c.preMatchedLocalizedOnly[targetChannel] = map[string]string{}
	}

	for _, source := range sourceRecords {
		sourceID := stringValue(source["id"])
		if sourceID == "" {
			continue
		}
		if _, ok := mapped[sourceID]; ok {
			continue // already matched
		}

		// Match by slug, then title_field_value
		var target map[string]any
		if slug := stringValue(source["slug"]); slug != "" {
			target = bySlug[slug]
		}
		if target == nil {
			if tfv := stringValue(source["title_field_value"]); tfv != "" {
				target = byTitle[tfv]
			}
		}
		if target == nil {
			continue
		}

		targetID := stringValue(target["id"])
		if targetID == "" {
			continue
		}

		defaultEqual := contentEqual(source, target, info)
		localizedEqual := localizedVariantsEqual(sourceID, targetID, localizedSourceRecords, localizedTargetRecords, info, c.locales)
		if defaultEqual && localizedEqual {
			// Content identical — skip
			mapped[sourceID] = targetID
			c.result.Items = append(c.result.Items, RecordCopyItem{
				Action:     "skip",
				Identifier: recordIdentifier(source),
				Resource:   targetChannel,
				SourceID:   sourceID,
				TargetID:   targetID,
				Localized:  localizedCopyItemsForSource(sourceID, localizedSourceRecords, info, c.locales, "skip"),
			})
		} else if defaultEqual {
			c.preMatchedLocalizedOnly[targetChannel][sourceID] = targetID
		} else {
			// Content differs — pre-match for fast upsert
			c.preMatched[targetChannel][sourceID] = targetID
		}
	}
}

// lookupPreMatched returns a pre-matched target ID for a source record.
func (c *recordCopier) lookupPreMatched(channel, sourceID string) (string, bool) {
	if c.preMatched == nil {
		return "", false
	}
	if m, ok := c.preMatched[channel]; ok {
		if targetID, ok := m[sourceID]; ok {
			return targetID, true
		}
	}
	return "", false
}

func (c *recordCopier) lookupPreMatchedLocalizedOnly(channel, sourceID string) (string, bool) {
	if c.preMatchedLocalizedOnly == nil {
		return "", false
	}
	if m, ok := c.preMatchedLocalizedOnly[channel]; ok {
		if targetID, ok := m[sourceID]; ok {
			return targetID, true
		}
	}
	return "", false
}

// contentEqual compares scalar fields between source and target, ignoring
// system fields, files, galleries, and references (which have different IDs).
func contentEqual(source, target map[string]any, info schemaInfo) bool {
	skipCompare := map[string]bool{
		"id": true, "_id": true, "created_at": true, "updated_at": true,
		"url": true, "entries_url": true, "short_id": true,
	}
	// Skip complex fields (files, galleries) — their IDs differ
	for _, f := range info.fileFields {
		skipCompare[f.Name] = true
	}
	for _, f := range info.galleryFields {
		skipCompare[f.Name] = true
	}
	for _, f := range info.customerFields {
		skipCompare[f.Name] = true
	}

	// Build set of reference field names for targeted comparison.
	refFields := map[string]bool{}
	for _, f := range info.referenceFields {
		refFields[f.Name] = true
	}

	for key, sourceVal := range source {
		if skipCompare[key] {
			continue
		}
		// Reference fields: only check if source has data but target is nil/empty.
		// This detects entries broken by a previous run that sent plain IDs.
		if refFields[key] {
			if refFieldEmpty(target[key]) && !refFieldEmpty(sourceVal) {
				return false
			}
			continue
		}
		targetVal := target[key]
		if w := compareScalarField("", key, sourceVal, targetVal); w != "" {
			return false
		}
	}
	return true
}

// refFieldEmpty returns true if a reference field value is nil or has no
// meaningful content. Handles both plain format (string/[]any) and rich format
// (Reference/Relation objects).
func refFieldEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case map[string]any:
		// Relation wrapper: check objects array
		if objs, ok := val["objects"].([]any); ok {
			return len(objs) == 0
		}
		// Reference object: check id
		return stringValue(val["id"]) == ""
	}
	return false
}
