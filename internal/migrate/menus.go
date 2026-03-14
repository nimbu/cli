package migrate

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

// MenuCopyItem describes one copied menu.
type MenuCopyItem struct {
	Slug   string `json:"slug"`
	Action string `json:"action"`
}

// MenuCopyResult reports menu copy results.
type MenuCopyResult struct {
	From  SiteRef        `json:"from"`
	To    SiteRef        `json:"to"`
	Query string         `json:"query"`
	Items []MenuCopyItem `json:"items,omitempty"`
}

// CopyMenus copies nested menu documents. When overwriteExisting is false, existing menus are skipped.
func CopyMenus(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, query string, overwriteExisting bool, media *MediaRewritePlan, dryRun bool) (MenuCopyResult, error) {
	menus, err := listMenuDocuments(ctx, fromClient, query)
	if err != nil {
		return MenuCopyResult{From: fromRef, To: toRef, Query: query}, err
	}
	result := MenuCopyResult{From: fromRef, To: toRef, Query: query}
	for i, menu := range menus {
		slug := api.MenuDocumentSlug(menu)
		emitStageItem(ctx, "Menus", slug, int64(i+1), int64(len(menus)))
		if slug == "" {
			continue
		}
		sanitizeMenuDocument(menu)
		if media != nil {
			media.RewriteValue("menus."+slug, menu)
		}
		var existing api.MenuDocument
		err := toClient.Get(ctx, "/menus/"+url.PathEscape(slug), &existing)
		switch {
		case err == nil:
			if !overwriteExisting {
				return result, fmt.Errorf("menu %s already exists; rerun with --force to overwrite", slug)
			}
			action := "update"
			if dryRun {
				action = "dry-run:" + action
			} else if _, err := api.PatchMenuDocument(ctx, toClient, slug, menu); err != nil {
				return result, fmt.Errorf("update menu %s: %w", slug, err)
			}
			result.Items = append(result.Items, MenuCopyItem{Slug: slug, Action: action})
		case api.IsNotFound(err):
			action := "create"
			if dryRun {
				action = "dry-run:" + action
			} else if err := toClient.Post(ctx, "/menus", menu, &existing); err != nil {
				return result, fmt.Errorf("create menu %s: %w", slug, err)
			}
			result.Items = append(result.Items, MenuCopyItem{Slug: slug, Action: action})
		default:
			return result, err
		}
	}
	return result, nil
}

func listMenuDocuments(ctx context.Context, client *api.Client, query string) ([]api.MenuDocument, error) {
	opts := []api.RequestOption{api.WithParam("nested", "1")}
	query = strings.TrimSpace(query)
	if query != "" && query != "*" {
		opts = append(opts, api.WithParam("slug", query))
	}
	return api.List[api.MenuDocument](ctx, client, "/menus", opts...)
}

func sanitizeMenuDocument(doc api.MenuDocument) {
	delete(doc, "id")
	delete(doc, "created_at")
	delete(doc, "updated_at")
	api.NormalizeMenuDocumentForWrite(doc)
	items, ok := doc["items"].([]any)
	if !ok {
		return
	}
	sanitizeMenuItems(items)
}

func sanitizeMenuItems(items []any) {
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		delete(item, "id")
		delete(item, "created_at")
		delete(item, "updated_at")
		children, ok := item["children"].([]any)
		if ok {
			sanitizeMenuItems(children)
		}
	}
}
