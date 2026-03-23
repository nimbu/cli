package migrate

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
)

func (c *recordCopier) remapReferences(payload map[string]any, info schemaInfo, identifier string) {
	for _, field := range info.referenceFields {
		value := payload[field.Name]
		if value == nil {
			continue
		}
		switch field.Type {
		case "belongs_to", "customer":
			sourceID := refSourceID(value)
			if sourceID == "" {
				continue
			}
			if targetID, ok := c.lookupMappedID(referenceClass(field), sourceID); ok {
				payload[field.Name] = targetID
			} else {
				delete(payload, field.Name)
				c.trackUnresolved(info.resource, identifier, "", field, sourceID)
			}
		case "belongs_to_many":
			sourceIDs := refSourceIDs(value)
			if len(sourceIDs) == 0 {
				continue
			}
			var kept []string
			var unresolved bool
			for _, sid := range sourceIDs {
				if targetID, ok := c.lookupMappedID(referenceClass(field), sid); ok {
					kept = append(kept, targetID)
				} else if sid != "" {
					unresolved = true
				}
			}
			if len(kept) > 0 {
				payload[field.Name] = kept
			} else {
				delete(payload, field.Name)
			}
			if unresolved {
				c.trackUnresolved(info.resource, identifier, "", field, "")
			}
		}
	}
	if ownerID := stringValue(payload["_owner"]); ownerID != "" {
		if targetID, ok := c.lookupMappedID("customers", ownerID); ok {
			payload["_owner"] = targetID
		} else if !c.options.CopyCustomers {
			delete(payload, "_owner")
		}
	}
}

// refSourceID extracts the source ID from a belongs_to value.
// Handles plain string ID (API default) and rich Reference object (client-version 2).
func refSourceID(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		return stringValue(v["id"])
	}
	return ""
}

// refSourceIDs extracts source IDs from a belongs_to_many value.
// Handles plain []string/[]any (API default) and rich Relation object (client-version 2).
func refSourceIDs(value any) []string {
	switch v := value.(type) {
	case []any:
		ids := make([]string, 0, len(v))
		for _, item := range v {
			switch it := item.(type) {
			case string:
				ids = append(ids, it)
			case map[string]any:
				if id := stringValue(it["id"]); id != "" {
					ids = append(ids, id)
				}
			}
		}
		return ids
	case map[string]any:
		// Relation wrapper: {__type: "Relation", objects: [...]}
		if objs, ok := v["objects"].([]any); ok {
			return refSourceIDs(objs)
		}
	}
	return nil
}

func (c *recordCopier) trackUnresolved(channel, identifier, targetID string, field api.CustomField, sourceRefID string) {
	c.unresolvedRefs = append(c.unresolvedRefs, unresolvedRef{
		Channel:    channel,
		Entry:      identifier,
		TargetID:   targetID,
		Field:      field.Name,
		FieldType:  field.Type,
		RefChannel: referenceClass(field),
	})
}

// UnresolvedWarnings formats unresolved references as human-readable warnings.
func (c *recordCopier) UnresolvedWarnings() []string {
	if len(c.unresolvedRefs) == 0 {
		return nil
	}
	var warnings []string
	for _, ref := range c.unresolvedRefs {
		w := fmt.Sprintf("channel=%s entry=%s field=%s: unresolved %s reference to %s",
			ref.Channel, ref.Entry, ref.Field, ref.FieldType, ref.RefChannel)
		if ref.TargetID != "" {
			w += fmt.Sprintf(" (targetID=%s)", ref.TargetID)
		}
		warnings = append(warnings, w)
	}
	return warnings
}

func (c *recordCopier) remapPendingSelfRefs(payload map[string]any, selfRefs []api.CustomField) {
	for _, field := range selfRefs {
		value := payload[field.Name]
		if value == nil {
			continue
		}
		switch field.Type {
		case "belongs_to", "customer":
			sourceID := refSourceID(value)
			if sourceID == "" {
				continue
			}
			if targetID, ok := c.lookupMappedID(referenceClass(field), sourceID); ok {
				payload[field.Name] = targetID
			} else {
				delete(payload, field.Name)
			}
		case "belongs_to_many":
			sourceIDs := refSourceIDs(value)
			if len(sourceIDs) == 0 {
				continue
			}
			var kept []string
			for _, sid := range sourceIDs {
				if targetID, ok := c.lookupMappedID(referenceClass(field), sid); ok {
					kept = append(kept, targetID)
				}
			}
			if len(kept) > 0 {
				payload[field.Name] = kept
			} else {
				delete(payload, field.Name)
			}
		}
	}
}

// extractDeferredRefs removes reference fields pointing to channels not yet in the mapping.
// These will be resolved after all channels are copied.
func (c *recordCopier) extractDeferredRefs(payload map[string]any, info schemaInfo) map[string]any {
	deferred := map[string]any{}
	for _, field := range info.referenceFields {
		refChannel := referenceClass(field)
		if refChannel == info.resource || refChannel == "customers" {
			continue
		}
		if _, hasMappings := c.mapping[refChannel]; hasMappings {
			continue
		}
		if v, ok := payload[field.Name]; ok && v != nil {
			deferred[field.Name] = v
			delete(payload, field.Name)
		}
	}
	return deferred
}

// resolveDeferredRefs remaps and updates all deferred cross-channel references.
func (c *recordCopier) resolveDeferredRefs(ctx context.Context) ([]string, error) {
	var warnings []string
	for _, d := range c.deferredRefs {
		if d.targetID == "" || len(d.fields) == 0 {
			continue
		}
		c.remapDeferredFields(d.channel, d.targetID, d.fields, d.refFields)
		if len(d.fields) == 0 {
			continue // all fields were unresolved and removed
		}
		if err := c.updateRecord(ctx, d.channel, d.targetID, d.fields); err != nil {
			if isRecoverableRecordError(err) {
				warnings = append(warnings, fmt.Sprintf("%s deferred-ref update %s: %v", d.channel, d.targetID, err))
				continue
			}
			if c.options.AllowErrors {
				warnings = append(warnings, fmt.Sprintf("%s deferred-ref update %s: %v", d.channel, d.targetID, err))
				continue
			}
			return warnings, err
		}
	}
	return warnings, nil
}

// remapDeferredFields remaps reference IDs in a deferred payload using the now-complete mapping.
func (c *recordCopier) remapDeferredFields(channel, targetID string, payload map[string]any, refFields []api.CustomField) {
	for _, field := range refFields {
		value := payload[field.Name]
		if value == nil {
			continue
		}
		switch field.Type {
		case "belongs_to", "customer":
			sourceID := refSourceID(value)
			if sourceID == "" {
				continue
			}
			if mappedID, ok := c.lookupMappedID(referenceClass(field), sourceID); ok {
				payload[field.Name] = mappedID
			} else {
				delete(payload, field.Name)
				c.trackUnresolved(channel, targetID, targetID, field, sourceID)
			}
		case "belongs_to_many":
			sourceIDs := refSourceIDs(value)
			if len(sourceIDs) == 0 {
				continue
			}
			var kept []string
			var unresolved bool
			for _, sid := range sourceIDs {
				if mappedID, ok := c.lookupMappedID(referenceClass(field), sid); ok {
					kept = append(kept, mappedID)
				} else if sid != "" {
					unresolved = true
				}
			}
			if len(kept) > 0 {
				payload[field.Name] = kept
			} else {
				delete(payload, field.Name)
			}
			if unresolved {
				c.trackUnresolved(channel, targetID, targetID, field, "")
			}
		}
	}
}

func extractSelfRefs(payload map[string]any, selfRefs []api.CustomField) map[string]any {
	values := map[string]any{}
	for _, field := range selfRefs {
		if value, ok := payload[field.Name]; ok {
			values[field.Name] = value
			delete(payload, field.Name)
		}
	}
	return values
}

func collectDependencyIDs(records []map[string]any, info schemaInfo) map[string]map[string]struct{} {
	out := map[string]map[string]struct{}{}
	for _, field := range info.referenceFields {
		target := referenceClass(field)
		if target == "" || target == "customers" || target == info.resource {
			continue
		}
		addReferenceIDs(out, target, records, field)
	}
	return out
}

func collectCustomerIDs(records []map[string]any, info schemaInfo) map[string]struct{} {
	out := map[string]struct{}{}
	for _, field := range info.customerFields {
		addReferenceIDs(map[string]map[string]struct{}{"customers": out}, "customers", records, field)
	}
	for _, record := range records {
		if owner := stringValue(record["_owner"]); owner != "" {
			out[owner] = struct{}{}
		}
	}
	return out
}

func addReferenceIDs(target map[string]map[string]struct{}, class string, records []map[string]any, field api.CustomField) {
	if target[class] == nil {
		target[class] = map[string]struct{}{}
	}
	for _, record := range records {
		value := record[field.Name]
		switch field.Type {
		case "belongs_to", "customer":
			if ref, ok := value.(map[string]any); ok {
				if id := stringValue(ref["id"]); id != "" {
					target[class][id] = struct{}{}
				}
			}
		case "belongs_to_many":
			if relation, ok := value.(map[string]any); ok {
				if objects, ok := relation["objects"].([]any); ok {
					for _, rawObject := range objects {
						if ref, ok := rawObject.(map[string]any); ok {
							if id := stringValue(ref["id"]); id != "" {
								target[class][id] = struct{}{}
							}
						}
					}
				}
			}
		}
	}
}
