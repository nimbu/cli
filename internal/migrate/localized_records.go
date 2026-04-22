package migrate

import (
	"context"
	"net/url"
	"sort"

	"github.com/nimbu/cli/internal/api"
)

func (c *recordCopier) listLocalizedRecords(ctx context.Context, channel string, ids map[string]struct{}, info schemaInfo) (map[string]map[string]map[string]any, error) {
	if channel == "customers" || len(info.localizedFields) == 0 || len(c.locales) == 0 {
		return nil, nil
	}
	out := make(map[string]map[string]map[string]any, len(c.locales))
	for _, locale := range c.locales {
		records, err := c.listRecordsWithOptions(ctx, channel, ids, api.WithContentLocale(locale))
		if err != nil {
			return nil, err
		}
		out[locale] = indexRecordsByID(records)
	}
	return out, nil
}

func (c *recordCopier) listTargetLocalizedRecords(ctx context.Context, channel string, info schemaInfo) map[string]map[string]map[string]any {
	if channel == "customers" || len(info.localizedFields) == 0 || len(c.locales) == 0 {
		return nil
	}
	if c.localizedTargetRecords == nil {
		c.localizedTargetRecords = map[string]map[string]map[string]map[string]any{}
	}
	if records := c.localizedTargetRecords[channel]; records != nil {
		return records
	}

	path := "/channels/" + url.PathEscape(channel) + "/entries"
	out := make(map[string]map[string]map[string]any, len(c.locales))
	for _, locale := range c.locales {
		records, err := api.List[map[string]any](ctx, c.toClient, path, api.WithContentLocale(locale))
		if err != nil {
			continue
		}
		out[locale] = indexRecordsByID(records)
	}
	c.localizedTargetRecords[channel] = out
	return out
}

func (c *recordCopier) updateLocalizedRecords(ctx context.Context, targetChannel, targetID, sourceID, identifier string, info schemaInfo, localizedRecords map[string]map[string]map[string]any) ([]RecordLocalizedCopyItem, error) {
	if targetChannel == "customers" || sourceID == "" || len(info.localizedFields) == 0 || len(localizedRecords) == 0 {
		return nil, nil
	}

	targetRecords := c.listTargetLocalizedRecords(ctx, targetChannel, info)
	var items []RecordLocalizedCopyItem
	for _, locale := range c.locales {
		source := localizedRecords[locale][sourceID]
		payload := localizedPayload(source, info)
		if len(payload) == 0 {
			continue
		}
		fields := sortedMapKeys(payload)
		action := "update"
		if targetID == "" {
			action = "create"
		} else if localizedPayloadEqual(payload, targetRecords[locale][targetID], info) {
			action = "skip"
		}
		items = append(items, RecordLocalizedCopyItem{Locale: locale, Action: action, Fields: fields})
		if action == "skip" || c.options.DryRun || targetID == "" {
			continue
		}

		if err := c.prepareAttachments(ctx, payload, info); err != nil {
			return items, err
		}
		flattenSelectFields(payload, info)
		c.remapReferences(payload, info, identifier)
		if c.options.Media != nil {
			c.options.Media.RewriteValue(info.resource, payload)
		}
		if err := c.updateRecordWithOptions(ctx, targetChannel, targetID, payload, api.WithContentLocale(locale)); err != nil {
			return items, err
		}
		if targetRecords != nil {
			if targetRecords[locale] == nil {
				targetRecords[locale] = map[string]map[string]any{}
			}
			targetRecords[locale][targetID] = mergeRecordFields(targetRecords[locale][targetID], payload)
		}
	}
	return items, nil
}

func localizedPayload(record map[string]any, info schemaInfo) map[string]any {
	if len(record) == 0 {
		return nil
	}
	payload := map[string]any{}
	for _, field := range info.localizedFields {
		value, ok := record[field.Name]
		if !ok {
			continue
		}
		payload[field.Name] = deepCopyValue(value)
	}
	return payload
}

func localizedPayloadEqual(payload, target map[string]any, info schemaInfo) bool {
	if len(payload) == 0 {
		return true
	}
	for _, field := range info.localizedFields {
		sourceVal, ok := payload[field.Name]
		if !ok {
			continue
		}
		if w := compareField("", field.Name, localizedFieldType(field), sourceVal, target[field.Name]); w != "" {
			return false
		}
	}
	return true
}

func localizedVariantsEqual(sourceID, targetID string, source, target map[string]map[string]map[string]any, info schemaInfo, locales []string) bool {
	if len(info.localizedFields) == 0 || len(locales) == 0 {
		return true
	}
	for _, locale := range locales {
		sourcePayload := localizedPayload(source[locale][sourceID], info)
		targetRecord := target[locale][targetID]
		if !localizedPayloadEqual(sourcePayload, targetRecord, info) {
			return false
		}
	}
	return true
}

func localizedCopyItemsForSource(sourceID string, localizedRecords map[string]map[string]map[string]any, info schemaInfo, locales []string, action string) []RecordLocalizedCopyItem {
	if sourceID == "" || len(info.localizedFields) == 0 || len(localizedRecords) == 0 {
		return nil
	}
	var items []RecordLocalizedCopyItem
	for _, locale := range locales {
		payload := localizedPayload(localizedRecords[locale][sourceID], info)
		if len(payload) == 0 {
			continue
		}
		items = append(items, RecordLocalizedCopyItem{
			Locale: locale,
			Action: action,
			Fields: sortedMapKeys(payload),
		})
	}
	return items
}

func localizedFieldType(field api.CustomField) string {
	if field.Type == "" {
		return ""
	}
	return field.Type
}

func indexRecordsByID(records []map[string]any) map[string]map[string]any {
	index := make(map[string]map[string]any, len(records))
	for _, record := range records {
		if id := stringValue(record["id"]); id != "" {
			index[id] = record
		}
	}
	return index
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mergeRecordFields(record, payload map[string]any) map[string]any {
	if record == nil {
		record = map[string]any{}
	}
	for key, value := range payload {
		record[key] = deepCopyValue(value)
	}
	return record
}
