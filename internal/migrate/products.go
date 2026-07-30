package migrate

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
)

// ProductCopyItem describes one copied product.
type ProductCopyItem struct {
	Slug      string                     `json:"slug"`
	Name      string                     `json:"name"`
	Action    string                     `json:"action"`
	Localized []ProductLocalizedCopyItem `json:"localized,omitempty"`
}

// ProductLocalizedCopyItem describes one locale-specific product update.
type ProductLocalizedCopyItem struct {
	Locale   string   `json:"locale"`
	Action   string   `json:"action"`
	Fields   []string `json:"fields,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ProductCopyResult reports product copy results.
type ProductCopyResult struct {
	From     SiteRef           `json:"from"`
	To       SiteRef           `json:"to"`
	Items    []ProductCopyItem `json:"items,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
}

// ProductCopyOptions controls product copy behavior.
type ProductCopyOptions struct {
	AllowErrors bool
	DryRun      bool
	Media       *MediaRewritePlan
	Upsert      string
}

// CopyProducts copies products between sites and returns an ID mapping for collection remapping.
func CopyProducts(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, opts ProductCopyOptions) (ProductCopyResult, map[string]string, error) {
	result := ProductCopyResult{From: fromRef, To: toRef}
	idMapping := map[string]string{}

	fields, err := api.GetProductCustomizations(ctx, fromClient)
	if err != nil {
		return result, idMapping, fmt.Errorf("get product schema: %w", err)
	}
	info := buildSchemaInfo("products", fields)
	sourceLocales, targetLocales, locales, localeWarnings, localeInfoReady, err := productLocalePlan(ctx, fromClient, toClient, fromRef, toRef)
	result.Warnings = append(result.Warnings, localeWarnings...)
	if err != nil {
		return result, idMapping, err
	}

	srcProducts, err := api.List[map[string]any](ctx, fromClient, "/products")
	if err != nil {
		return result, idMapping, fmt.Errorf("list source products: %w", err)
	}

	dstProducts, err := api.List[map[string]any](ctx, toClient, "/products")
	if err != nil {
		return result, idMapping, fmt.Errorf("list target products: %w", err)
	}
	defaultSourceProducts := indexRecordsByID(srcProducts)
	localizedProducts := map[string]map[string]map[string]any{}
	localizedTargets := map[string]map[string]map[string]any{}
	if localeInfoReady {
		localizedProducts[sourceLocales.DefaultLocale] = defaultSourceProducts
		sourceFetchLocales := locales
		if targetLocales.DefaultLocale != sourceLocales.DefaultLocale {
			sourceFetchLocales = append(append([]string{}, sourceFetchLocales...), targetLocales.DefaultLocale)
		}
		sourceFetchLocales = localesExcept(sourceFetchLocales, sourceLocales.DefaultLocale)
		localizedProducts, localeWarnings, err = mergeLocalizedProducts(ctx, fromClient, localizedProducts, sourceFetchLocales, opts.AllowErrors, targetLocales.DefaultLocale)
		result.Warnings = append(result.Warnings, localeWarnings...)
		if err != nil {
			return result, idMapping, err
		}
		localizedTargets, localeWarnings, err = listLocalizedProducts(ctx, toClient, locales, opts.AllowErrors, "target")
		result.Warnings = append(result.Warnings, localeWarnings...)
		if err != nil {
			return result, idMapping, err
		}
	}
	targetBySlug := make(map[string]map[string]any, len(dstProducts))
	for _, p := range dstProducts {
		if slug := stringValue(p["slug"]); slug != "" {
			targetBySlug[slug] = p
		}
	}

	for i, src := range srcProducts {
		emitStageItem(ctx, "Products", stringValue(src["slug"]), int64(i+1), int64(len(srcProducts)))
		sourceID := stringValue(src["id"])
		slug := stringValue(src["slug"])
		if slug == "" {
			continue
		}

		baseLocalized := src
		if localeInfoReady {
			baseLocalized = localizedProducts[targetLocales.DefaultLocale][sourceID]
			if len(baseLocalized) == 0 {
				return result, idMapping, fmt.Errorf("product %s is unavailable in target default locale %q on source site", slug, targetLocales.DefaultLocale)
			}
		}
		payload, payloadWarnings := baseProductPayload(src, baseLocalized, info)
		for _, warning := range payloadWarnings {
			result.Warnings = append(result.Warnings, fmt.Sprintf("product %s target default locale: %s", slug, warning))
		}
		slug = stringValue(payload["slug"])
		name := stringValue(payload["name"])
		if slug == "" {
			continue
		}

		if err := prepareProductAttachments(ctx, fromClient, payload, info); err != nil {
			if opts.AllowErrors {
				continue
			}
			return result, idMapping, err
		}
		flattenSelectFields(payload, info)

		if opts.Media != nil {
			opts.Media.RewriteValue("products."+slug, payload)
		}

		if existing, ok := targetBySlug[slug]; ok {
			remapProductImageIDs(payload, existing)
			targetID := stringValue(existing["id"])
			action := "update"
			if opts.DryRun {
				action = "dry-run:" + action
			} else {
				path := "/products/" + url.PathEscape(targetID)
				if err := toClient.Put(ctx, path, payload, nil); err != nil {
					if opts.AllowErrors {
						continue
					}
					return result, idMapping, fmt.Errorf("update product %s: %w", slug, err)
				}
			}
			localized, warnings, err := copyLocalizedProduct(ctx, fromClient, toClient, sourceID, targetID, existing, info, locales, localizedProducts, localizedTargets, false, opts)
			result.Warnings = append(result.Warnings, warnings...)
			if err != nil {
				return result, idMapping, err
			}
			if sourceID != "" && targetID != "" {
				idMapping[sourceID] = targetID
			}
			result.Items = append(result.Items, ProductCopyItem{Slug: slug, Name: name, Action: action, Localized: localized})
		} else {
			remapProductImageIDs(payload, nil)
			action := "create"
			var created map[string]any
			if opts.DryRun {
				action = "dry-run:" + action
			} else {
				if err := toClient.Post(ctx, "/products", payload, &created); err != nil {
					if opts.AllowErrors {
						continue
					}
					return result, idMapping, fmt.Errorf("create product %s: %w", slug, err)
				}
				targetID := stringValue(created["id"])
				if sourceID != "" && targetID != "" {
					idMapping[sourceID] = targetID
				}
			}
			targetID := stringValue(created["id"])
			if !opts.DryRun && targetID != "" && localizedVariantsNeedTargetDetails(sourceID, locales, localizedProducts, created) {
				var hydrated map[string]any
				path := "/products/" + url.PathEscape(targetID)
				if err := toClient.Get(ctx, path, &hydrated); err != nil {
					warning := fmt.Sprintf("hydrate created product %s before localized updates: %v", slug, err)
					if !opts.AllowErrors {
						return result, idMapping, fmt.Errorf("%s: %w", warning, err)
					}
					localized, warnings := skippedLocalizedProductItems(sourceID, locales, localizedProducts, warning)
					result.Warnings = append(result.Warnings, warning)
					result.Warnings = append(result.Warnings, warnings...)
					result.Items = append(result.Items, ProductCopyItem{Slug: slug, Name: name, Action: action, Localized: localized})
					continue
				}
				created = hydrated
			}
			localizedTarget := created
			if opts.DryRun {
				localizedTarget = payload
			}
			localized, warnings, err := copyLocalizedProduct(ctx, fromClient, toClient, sourceID, targetID, localizedTarget, info, locales, localizedProducts, localizedTargets, opts.DryRun, opts)
			result.Warnings = append(result.Warnings, warnings...)
			if err != nil {
				return result, idMapping, err
			}
			result.Items = append(result.Items, ProductCopyItem{Slug: slug, Name: name, Action: action, Localized: localized})
		}
	}

	return result, idMapping, nil
}

func prepareProductAttachments(ctx context.Context, client *api.Client, payload map[string]any, info schemaInfo) error {
	for _, field := range info.fileFields {
		file, ok := payload[field.Name].(map[string]any)
		if !ok {
			continue
		}
		if err := embedFileFromClient(ctx, client, file); err != nil {
			return err
		}
	}
	for _, field := range info.galleryFields {
		gallery, ok := payload[field.Name].(map[string]any)
		if !ok {
			continue
		}
		images, ok := gallery["images"].([]any)
		if !ok {
			continue
		}
		for _, rawImage := range images {
			image, ok := rawImage.(map[string]any)
			if !ok {
				continue
			}
			delete(image, "id")
			file, ok := image["file"].(map[string]any)
			if !ok {
				continue
			}
			if err := embedFileFromClient(ctx, client, file); err != nil {
				return err
			}
		}
	}
	return nil
}

// remapProductImageIDs matches source product images to target images by
// filename. Matched images get the target's ID (update); unmatched images
// have their ID stripped (create new). When target is nil (new product),
// all IDs are stripped.
func remapProductImageIDs(payload map[string]any, target map[string]any) {
	rawImages, ok := payload["images"].([]any)
	if !ok {
		return
	}

	// Build filename → target image IDs lookup from the existing product.
	// A slice per filename handles duplicates: IDs are consumed in order,
	// giving a position-based fallback when filenames repeat.
	targetByFilename := map[string][]string{}
	if target != nil {
		if targetImages, ok := target["images"].([]any); ok {
			for _, rawImg := range targetImages {
				img, ok := rawImg.(map[string]any)
				if !ok {
					continue
				}
				id := stringValue(img["id"])
				filename := productImageFilename(img)
				if id != "" && filename != "" {
					targetByFilename[filename] = append(targetByFilename[filename], id)
				}
			}
		}
	}

	for _, rawImg := range rawImages {
		img, ok := rawImg.(map[string]any)
		if !ok {
			continue
		}
		filename := productImageFilename(img)
		if ids := targetByFilename[filename]; len(ids) > 0 {
			img["id"] = ids[0]
			targetByFilename[filename] = ids[1:]
		} else {
			delete(img, "id")
		}
	}
}

// productImageFilename extracts the filename from a product image's file sub-object.
func productImageFilename(img map[string]any) string {
	file, ok := img["file"].(map[string]any)
	if !ok {
		return ""
	}
	return stringValue(file["filename"])
}
