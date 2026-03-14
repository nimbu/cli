package migrate

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
)

// CollectionCopyItem describes one copied collection.
type CollectionCopyItem struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Action string `json:"action"`
}

// CollectionCopyResult reports collection copy results.
type CollectionCopyResult struct {
	From  SiteRef              `json:"from"`
	To    SiteRef              `json:"to"`
	Items []CollectionCopyItem `json:"items,omitempty"`
}

// CollectionCopyOptions controls collection copy behavior.
type CollectionCopyOptions struct {
	AllowErrors    bool
	DryRun         bool
	Media          *MediaRewritePlan
	ProductMapping map[string]string
}

// CopyCollections copies product collections between sites.
func CopyCollections(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, opts CollectionCopyOptions) (CollectionCopyResult, error) {
	result := CollectionCopyResult{From: fromRef, To: toRef}

	srcCollections, err := api.List[map[string]any](ctx, fromClient, "/collections")
	if err != nil {
		return result, fmt.Errorf("list source collections: %w", err)
	}

	dstCollections, err := api.List[map[string]any](ctx, toClient, "/collections")
	if err != nil {
		return result, fmt.Errorf("list target collections: %w", err)
	}
	targetBySlug := make(map[string]map[string]any, len(dstCollections))
	for _, c := range dstCollections {
		if slug := stringValue(c["slug"]); slug != "" {
			targetBySlug[slug] = c
		}
	}

	for i, src := range srcCollections {
		emitStageItem(ctx, "Collections", stringValue(src["slug"]), int64(i+1), int64(len(srcCollections)))
		slug := stringValue(src["slug"])
		name := stringValue(src["name"])
		if slug == "" {
			continue
		}

		payload := deepCopyMap(src)
		stripSystemFields(payload)
		delete(payload, "product_count")
		delete(payload, "featured_image")

		remapCollectionProductIDs(payload, opts.ProductMapping)
		rewriteCollectionImages(payload)

		if opts.Media != nil {
			opts.Media.RewriteValue("collections."+slug, payload)
		}

		if existing, ok := targetBySlug[slug]; ok {
			action := "update"
			if opts.DryRun {
				action = "dry-run:" + action
			} else {
				targetID := stringValue(existing["id"])
				path := "/collections/" + url.PathEscape(targetID)
				if err := toClient.Put(ctx, path, payload, nil); err != nil {
					if opts.AllowErrors {
						continue
					}
					return result, fmt.Errorf("update collection %s: %w", slug, err)
				}
			}
			result.Items = append(result.Items, CollectionCopyItem{Slug: slug, Name: name, Action: action})
		} else {
			action := "create"
			if opts.DryRun {
				action = "dry-run:" + action
			} else {
				if err := toClient.Post(ctx, "/collections", payload, nil); err != nil {
					if opts.AllowErrors {
						continue
					}
					return result, fmt.Errorf("create collection %s: %w", slug, err)
				}
			}
			result.Items = append(result.Items, CollectionCopyItem{Slug: slug, Name: name, Action: action})
		}
	}

	return result, nil
}

func remapCollectionProductIDs(payload map[string]any, mapping map[string]string) {
	if len(mapping) == 0 {
		return
	}
	rawIDs, ok := payload["product_ids"].([]any)
	if !ok {
		return
	}
	remapped := make([]any, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		id := stringValue(rawID)
		if targetID, ok := mapping[id]; ok {
			remapped = append(remapped, targetID)
		} else {
			remapped = append(remapped, rawID)
		}
	}
	payload["product_ids"] = remapped
}

func rewriteCollectionImages(payload map[string]any) {
	rawImages, ok := payload["images"].([]any)
	if !ok {
		return
	}
	for _, rawImg := range rawImages {
		img, ok := rawImg.(map[string]any)
		if !ok {
			continue
		}
		delete(img, "id")
	}
}
