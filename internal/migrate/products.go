package migrate

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
)

// ProductCopyItem describes one copied product.
type ProductCopyItem struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Action string `json:"action"`
}

// ProductCopyResult reports product copy results.
type ProductCopyResult struct {
	From  SiteRef           `json:"from"`
	To    SiteRef           `json:"to"`
	Items []ProductCopyItem `json:"items,omitempty"`
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

	srcProducts, err := api.List[map[string]any](ctx, fromClient, "/products")
	if err != nil {
		return result, idMapping, fmt.Errorf("list source products: %w", err)
	}

	dstProducts, err := api.List[map[string]any](ctx, toClient, "/products")
	if err != nil {
		return result, idMapping, fmt.Errorf("list target products: %w", err)
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
		name := stringValue(src["name"])
		if slug == "" {
			continue
		}

		payload := deepCopyMap(src)
		stripSystemFields(payload)

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
			if sourceID != "" && targetID != "" {
				idMapping[sourceID] = targetID
			}
			result.Items = append(result.Items, ProductCopyItem{Slug: slug, Name: name, Action: action})
		} else {
			action := "create"
			if opts.DryRun {
				action = "dry-run:" + action
			} else {
				var created map[string]any
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
			result.Items = append(result.Items, ProductCopyItem{Slug: slug, Name: name, Action: action})
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
