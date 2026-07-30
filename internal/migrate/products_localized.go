package migrate

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
)

func productLocalePlan(
	ctx context.Context,
	fromClient, toClient *api.Client,
	fromRef, toRef SiteRef,
) (siteLocaleInfo, siteLocaleInfo, []string, []string, bool, error) {
	source, sourceErr := fetchSiteLocaleInfo(ctx, fromClient, fromRef.Site)
	target, targetErr := fetchSiteLocaleInfo(ctx, toClient, toRef.Site)
	var warnings []string
	if sourceErr != nil {
		warnings = append(warnings, fmt.Sprintf("source site locales fetch failed: %v", sourceErr))
	}
	if targetErr != nil {
		warnings = append(warnings, fmt.Sprintf("target site locales fetch failed: %v", targetErr))
	}
	if sourceErr != nil {
		return source, target, nil, warnings, false, fmt.Errorf("source site locales fetch failed: %w", sourceErr)
	}
	if targetErr != nil {
		return source, target, nil, warnings, false, fmt.Errorf("target site locales fetch failed: %w", targetErr)
	}
	if !source.ExplicitDefault {
		return source, target, nil, warnings, false, fmt.Errorf("source site explicit default_locale is required for product copy")
	}
	if !target.ExplicitDefault {
		return source, target, nil, warnings, false, fmt.Errorf("target site explicit default_locale is required for product copy")
	}
	if !containsLocale(source.Locales, target.DefaultLocale) {
		return source, target, nil, warnings, true, fmt.Errorf(
			"target default locale %q is not available on source site; add it to the source site or align the default locales",
			target.DefaultLocale,
		)
	}
	return source, target, sharedLocalesExceptTargetDefault(source.Locales, target.Locales, target.DefaultLocale), warnings, true, nil
}

func sharedLocalesExceptTargetDefault(source, target []string, targetDefault string) []string {
	return sharedNonDefaultLocales(source, target, targetDefault)
}

func containsLocale(locales []string, want string) bool {
	for _, locale := range locales {
		if locale == want {
			return true
		}
	}
	return false
}

func localesExcept(locales []string, excluded string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, locale := range locales {
		if locale == "" || locale == excluded {
			continue
		}
		if _, ok := seen[locale]; ok {
			continue
		}
		seen[locale] = struct{}{}
		out = append(out, locale)
	}
	return out
}

func mergeLocalizedProducts(
	ctx context.Context,
	client *api.Client,
	existing map[string]map[string]map[string]any,
	locales []string,
	allowErrors bool,
	requiredLocale string,
) (map[string]map[string]map[string]any, []string, error) {
	var allWarnings []string
	for _, locale := range locales {
		allowLocaleError := allowErrors && locale != requiredLocale
		records, warnings, err := listLocalizedProducts(ctx, client, []string{locale}, allowLocaleError, "source")
		allWarnings = append(allWarnings, warnings...)
		if err != nil {
			if locale == requiredLocale {
				return existing, allWarnings, fmt.Errorf("fetch source products for target default locale %q: %w", locale, err)
			}
			return existing, allWarnings, err
		}
		if records[locale] != nil {
			existing[locale] = records[locale]
		}
	}
	return existing, allWarnings, nil
}

func listLocalizedProducts(ctx context.Context, client *api.Client, locales []string, allowErrors bool, side string) (map[string]map[string]map[string]any, []string, error) {
	out := make(map[string]map[string]map[string]any, len(locales))
	var warnings []string
	for _, locale := range locales {
		products, err := api.List[map[string]any](ctx, client, "/products", api.WithContentLocale(locale))
		if err != nil {
			warning := fmt.Sprintf("%s products locale=%s fetch failed: %v", side, locale, err)
			if !allowErrors {
				return out, warnings, fmt.Errorf("%s: %w", warning, err)
			}
			warnings = append(warnings, warning)
			continue
		}
		out[locale] = indexRecordsByID(products)
	}
	return out, warnings, nil
}

func copyLocalizedProduct(
	ctx context.Context,
	sourceClient, targetClient *api.Client,
	sourceID, targetID string,
	target map[string]any,
	info schemaInfo,
	locales []string,
	source, destination map[string]map[string]map[string]any,
	dryRunCreate bool,
	opts ProductCopyOptions,
) ([]ProductLocalizedCopyItem, []string, error) {
	if sourceID == "" || len(locales) == 0 {
		return nil, nil, nil
	}

	var items []ProductLocalizedCopyItem
	var warnings []string
	for _, locale := range locales {
		record := source[locale][sourceID]
		payload, payloadWarnings := localizedProductPayload(record, target, info)
		if dryRunCreate {
			payload, payloadWarnings = localizedProductCreatePlanPayload(record, target, info)
		}
		for _, warning := range payloadWarnings {
			warnings = append(warnings, fmt.Sprintf("product source_id=%s locale=%s: %s", sourceID, locale, warning))
		}
		if len(payload) == 0 {
			if len(payloadWarnings) > 0 {
				items = append(items, ProductLocalizedCopyItem{
					Locale:   locale,
					Action:   "skip:error",
					Warnings: payloadWarnings,
				})
			}
			continue
		}
		if err := prepareProductAttachments(ctx, sourceClient, payload, info); err != nil {
			warning := fmt.Sprintf("product source_id=%s locale=%s attachment preparation failed: %v", sourceID, locale, err)
			if !opts.AllowErrors {
				return items, warnings, fmt.Errorf("%s: %w", warning, err)
			}
			warnings = append(warnings, warning)
			items = append(items, ProductLocalizedCopyItem{
				Locale:   locale,
				Action:   "skip:error",
				Fields:   sortedMapKeys(payload),
				Warnings: append(payloadWarnings, warning),
			})
			continue
		}
		flattenSelectFields(payload, info)
		if opts.Media != nil {
			opts.Media.RewriteValue("products."+stringValue(record["slug"])+"."+locale, payload)
		}
		item := ProductLocalizedCopyItem{
			Locale:   locale,
			Fields:   sortedMapKeys(payload),
			Warnings: payloadWarnings,
		}
		if localizedProductPayloadMatches(payload, destination[locale][targetID]) {
			item.Action = "skip"
			items = append(items, item)
			continue
		}
		if opts.DryRun {
			item.Action = "dry-run:update"
			items = append(items, item)
			continue
		}
		if targetID == "" {
			warning := fmt.Sprintf("product source_id=%s locale=%s: missing target product id; localized update skipped", sourceID, locale)
			warnings = append(warnings, warning)
			item.Warnings = append(item.Warnings, warning)
			item.Action = "skip:error"
			items = append(items, item)
			continue
		}
		path := "/products/" + url.PathEscape(targetID)
		if err := targetClient.Put(ctx, path, payload, nil, api.WithContentLocale(locale)); err != nil {
			warning := fmt.Sprintf("product source_id=%s locale=%s update failed: %v", sourceID, locale, err)
			if !opts.AllowErrors {
				return items, warnings, fmt.Errorf("%s: %w", warning, err)
			}
			warnings = append(warnings, warning)
			item.Warnings = append(item.Warnings, warning)
			item.Action = "skip:error"
			items = append(items, item)
			continue
		}
		item.Action = "update"
		items = append(items, item)
	}
	return items, warnings, nil
}

func skippedLocalizedProductItems(
	sourceID string,
	locales []string,
	source map[string]map[string]map[string]any,
	warning string,
) ([]ProductLocalizedCopyItem, []string) {
	var items []ProductLocalizedCopyItem
	var warnings []string
	for _, locale := range locales {
		record := source[locale][sourceID]
		if len(record) == 0 {
			continue
		}
		itemWarning := fmt.Sprintf("product source_id=%s locale=%s: %s", sourceID, locale, warning)
		warnings = append(warnings, itemWarning)
		items = append(items, ProductLocalizedCopyItem{
			Locale:   locale,
			Action:   "skip:error",
			Fields:   sortedMapKeys(record),
			Warnings: []string{warning},
		})
	}
	return items, warnings
}

func localizedVariantsNeedTargetDetails(
	sourceID string,
	locales []string,
	source map[string]map[string]map[string]any,
	target map[string]any,
) bool {
	targetBySKU := productVariantsBySKU(anySlice(target["variants"]))
	for _, locale := range locales {
		variants := anySlice(source[locale][sourceID]["variants"])
		sourceCounts := productVariantSKUCounts(variants)
		for _, raw := range variants {
			variant, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if _, hasLabel := variant["label"]; !hasLabel {
				continue
			}
			sku := stringValue(variant["sku"])
			if sku == "" || sourceCounts[sku] != 1 {
				continue
			}
			matches := targetBySKU[sku]
			if len(matches) != 1 || stringValue(matches[0]["id"]) == "" {
				return true
			}
		}
	}
	return false
}

func anySlice(value any) []any {
	values, _ := value.([]any)
	return values
}
