package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ProjectLocale returns a display-only copy with translations[locale]
// recursively overlaid onto each containing object.
func ProjectLocale(value any, locale string) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode locale projection: %w", err)
	}

	var copied any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&copied); err != nil {
		return nil, fmt.Errorf("decode locale projection: %w", err)
	}
	return projectLocaleValue(copied, locale), nil
}

func projectLocaleValue(value any, locale string) any {
	switch value := value.(type) {
	case map[string]any:
		if translations, ok := value["translations"].(map[string]any); ok {
			if localized := localizedTranslation(translations, locale); localized != nil {
				for key, translated := range localized {
					value[key] = translated
				}
			}
		}
		for key, nested := range value {
			value[key] = projectLocaleValue(nested, locale)
		}
		return value
	case []any:
		for i, nested := range value {
			value[i] = projectLocaleValue(nested, locale)
		}
		return value
	default:
		return value
	}
}

func localizedTranslation(translations map[string]any, locale string) map[string]any {
	if localized, ok := translations[locale].(map[string]any); ok {
		return localized
	}

	normalized := normalizeProjectionLocale(locale)
	if normalized == "" {
		return nil
	}

	keys := make([]string, 0, len(translations))
	for candidate := range translations {
		keys = append(keys, candidate)
	}
	sort.Strings(keys)
	for _, candidate := range keys {
		if normalizeProjectionLocale(candidate) != normalized {
			continue
		}
		if localized, ok := translations[candidate].(map[string]any); ok {
			return localized
		}
	}
	return nil
}

func normalizeProjectionLocale(locale string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(locale), "_", "-"))
}
