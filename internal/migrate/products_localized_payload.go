package migrate

import (
	"fmt"
	"reflect"
)

func localizedProductCreatePlanPayload(record, base map[string]any, info schemaInfo) (map[string]any, []string) {
	if len(record) == 0 {
		return nil, nil
	}
	payload := localizedProductTopLevelPayload(record, info)
	variants, warnings := localizedProductVariantsForCreatePlan(record["variants"], base["variants"])
	if len(variants) > 0 {
		payload["variants"] = variants
	}
	return payload, warnings
}

func localizedProductVariantsForCreatePlan(source, base any) ([]any, []string) {
	sourceVariants := anySlice(source)
	baseVariants := anySlice(base)
	sourceSKUCounts := productVariantSKUCounts(sourceVariants)
	baseBySKU := productVariantsBySKU(baseVariants)
	var variants []any
	var warnings []string
	warned := map[string]struct{}{}
	for _, raw := range sourceVariants {
		variant, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		label, hasLabel := variant["label"]
		if !hasLabel {
			continue
		}
		sku := stringValue(variant["sku"])
		if sku == "" {
			if _, seen := warned["source:<blank>"]; !seen {
				warnings = append(warnings, "source variant has blank SKU; localized label skipped")
				warned["source:<blank>"] = struct{}{}
			}
			continue
		}
		if sourceSKUCounts[sku] != 1 {
			if _, seen := warned["source:"+sku]; !seen {
				warnings = append(warnings, fmt.Sprintf("source variant SKU %q is duplicated; localized labels skipped", sku))
				warned["source:"+sku] = struct{}{}
			}
			continue
		}
		matches := baseBySKU[sku]
		if len(matches) != 1 {
			if _, seen := warned["base:"+sku]; !seen {
				reason := "missing"
				if len(matches) > 1 {
					reason = "duplicated"
				}
				warnings = append(warnings, fmt.Sprintf("base variant SKU %q is %s; localized label skipped", sku, reason))
				warned["base:"+sku] = struct{}{}
			}
			continue
		}
		variants = append(variants, map[string]any{"sku": sku, "label": deepCopyValue(label)})
	}
	return variants, warnings
}

func localizedProductPayload(record, target map[string]any, info schemaInfo) (map[string]any, []string) {
	if len(record) == 0 {
		return nil, nil
	}
	payload := localizedProductTopLevelPayload(record, info)
	variants, warnings := localizedProductVariants(record["variants"], target["variants"])
	if len(variants) > 0 {
		payload["variants"] = variants
	}
	return payload, warnings
}

func localizedProductTopLevelPayload(record map[string]any, info schemaInfo) map[string]any {
	payload := localizedPayload(record, info)
	for _, key := range []string{
		"name",
		"variant_name",
		"description",
		"slug",
		"seo_title",
		"seo_description",
		"seo_keywords",
	} {
		if value, ok := record[key]; ok {
			payload[key] = deepCopyValue(value)
		}
	}
	return payload
}

func baseProductPayload(base, localized map[string]any, info schemaInfo) (map[string]any, []string) {
	payload := deepCopyMap(base)
	stripSystemFields(payload)
	stripProductTranslations(payload)
	for key, value := range localizedProductTopLevelPayload(localized, info) {
		payload[key] = value
	}

	variants, warnings := localizedProductVariantsForCreatePlan(localized["variants"], payload["variants"])
	labelsBySKU := map[string]any{}
	for _, raw := range variants {
		if variant, ok := raw.(map[string]any); ok {
			labelsBySKU[stringValue(variant["sku"])] = variant["label"]
		}
	}
	if payloadVariants, ok := payload["variants"].([]any); ok {
		for _, raw := range payloadVariants {
			if variant, ok := raw.(map[string]any); ok {
				if label, ok := labelsBySKU[stringValue(variant["sku"])]; ok {
					variant["label"] = label
				}
			}
		}
	}
	stripProductTranslations(payload)
	return payload, warnings
}

func localizedProductVariants(source, target any) ([]any, []string) {
	sourceVariants, ok := source.([]any)
	if !ok {
		return nil, nil
	}
	targetVariants, _ := target.([]any)
	sourceSKUCounts := productVariantSKUCounts(sourceVariants)
	targetBySKU := productVariantsBySKU(targetVariants)
	var variants []any
	var warnings []string
	warned := map[string]struct{}{}
	for _, raw := range sourceVariants {
		variant, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		label, hasLabel := variant["label"]
		if !hasLabel {
			continue
		}
		sku := stringValue(variant["sku"])
		warningKey := "source:" + sku
		if sku == "" {
			warningKey = "source:<blank>"
			if _, seen := warned[warningKey]; !seen {
				warnings = append(warnings, "source variant has blank SKU; localized label skipped")
				warned[warningKey] = struct{}{}
			}
			continue
		}
		if sourceSKUCounts[sku] != 1 {
			if _, seen := warned[warningKey]; !seen {
				warnings = append(warnings, fmt.Sprintf("source variant SKU %q is duplicated; localized labels skipped", sku))
				warned[warningKey] = struct{}{}
			}
			continue
		}
		targetMatches := targetBySKU[sku]
		if len(targetMatches) != 1 {
			warningKey = "target:" + sku
			if _, seen := warned[warningKey]; !seen {
				reason := "missing"
				if len(targetMatches) > 1 {
					reason = "duplicated"
				}
				warnings = append(warnings, fmt.Sprintf("target variant SKU %q is %s; localized label skipped", sku, reason))
				warned[warningKey] = struct{}{}
			}
			continue
		}
		targetID := stringValue(targetMatches[0]["id"])
		if targetID == "" {
			warnings = append(warnings, fmt.Sprintf("target variant SKU %q has no ID; localized label skipped", sku))
			continue
		}
		variants = append(variants, map[string]any{"id": targetID, "label": deepCopyValue(label)})
	}
	return variants, warnings
}

func productVariantSKUCounts(variants []any) map[string]int {
	counts := map[string]int{}
	for _, raw := range variants {
		if variant, ok := raw.(map[string]any); ok {
			counts[stringValue(variant["sku"])]++
		}
	}
	return counts
}

func productVariantsBySKU(variants []any) map[string][]map[string]any {
	out := map[string][]map[string]any{}
	for _, raw := range variants {
		if variant, ok := raw.(map[string]any); ok {
			sku := stringValue(variant["sku"])
			if sku != "" {
				out[sku] = append(out[sku], variant)
			}
		}
	}
	return out
}

func stripProductTranslations(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "translations")
		for _, child := range typed {
			stripProductTranslations(child)
		}
	case []any:
		for _, child := range typed {
			stripProductTranslations(child)
		}
	}
}

func localizedProductPayloadMatches(payload, target map[string]any) bool {
	if len(payload) == 0 || len(target) == 0 {
		return false
	}
	return localizedValueSubset(payload, target)
}

func localizedValueSubset(source, target any) bool {
	switch typed := source.(type) {
	case map[string]any:
		targetMap, ok := target.(map[string]any)
		if !ok {
			return false
		}
		for key, value := range typed {
			targetValue, exists := targetMap[key]
			if !exists || !localizedValueSubset(value, targetValue) {
				return false
			}
		}
		return true
	case []any:
		targetSlice, ok := target.([]any)
		if !ok {
			return false
		}
		if localizedSliceHasIDs(typed) {
			targetByID := map[string]any{}
			for _, item := range targetSlice {
				if itemMap, ok := item.(map[string]any); ok {
					if id := stringValue(itemMap["id"]); id != "" {
						targetByID[id] = item
					}
				}
			}
			for _, item := range typed {
				itemMap := item.(map[string]any)
				if !localizedValueSubset(item, targetByID[stringValue(itemMap["id"])]) {
					return false
				}
			}
			return true
		}
		if len(typed) != len(targetSlice) {
			return false
		}
		for index := range typed {
			if !localizedValueSubset(typed[index], targetSlice[index]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(source, target)
	}
}

func localizedSliceHasIDs(values []any) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok || stringValue(item["id"]) == "" {
			return false
		}
	}
	return true
}
