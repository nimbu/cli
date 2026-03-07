package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMenuStatsCountsNestedItems(t *testing.T) {
	doc := MenuDocument{
		"items": []any{
			map[string]any{
				"title": "Home",
			},
			map[string]any{
				"title": "Shop",
				"children": []any{
					map[string]any{"title": "Wine"},
					map[string]any{
						"title": "Gifts",
						"children": []any{
							map[string]any{"title": "Boxes"},
						},
					},
				},
			},
		},
	}

	stats := MenuStats(doc)
	if stats.ItemCount != 5 {
		t.Fatalf("expected 5 menu items, got %d", stats.ItemCount)
	}
	if stats.MaxDepth != 3 {
		t.Fatalf("expected max depth 3, got %d", stats.MaxDepth)
	}
}

func TestNormalizeMenuDocumentForWriteStripsTargetPageOnly(t *testing.T) {
	doc := MenuDocument{
		"items": []any{
			map[string]any{
				"title":       "Home",
				"target_page": "home",
				"url":         "/",
				"children": []any{
					map[string]any{
						"title":       "Wine",
						"target_page": "wine",
						"url":         "/wine",
					},
				},
			},
		},
	}

	NormalizeMenuDocumentForWrite(doc)

	root := doc["items"].([]any)[0].(map[string]any)
	if _, ok := root["target_page"]; ok {
		t.Fatal("expected target_page to be removed from root item")
	}
	if root["url"] != "/" {
		t.Fatalf("expected unrelated fields preserved, got %#v", root["url"])
	}
	child := root["children"].([]any)[0].(map[string]any)
	if _, ok := child["target_page"]; ok {
		t.Fatal("expected target_page to be removed from child item")
	}
}

func TestGetMenuDocumentFallsBackToNestedList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/menus/main":
			_, _ = w.Write([]byte(`{"id":"m1","slug":"main","handle":"main","name":"Main"}`))
		case "/menus":
			if got := r.URL.Query().Get("nested"); got != "1" {
				t.Fatalf("expected nested=1 query, got %q", got)
			}
			if got := r.URL.Query().Get("slug"); got != "main" {
				t.Fatalf("expected slug=main query, got %q", got)
			}
			_, _ = w.Write([]byte(`[{"id":"m1","slug":"main","handle":"main","name":"Main","items":[{"title":"Home"}]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	doc, err := GetMenuDocument(context.Background(), New(srv.URL, ""), "main")
	if err != nil {
		t.Fatalf("get menu document: %v", err)
	}
	if !MenuDocumentHasItems(doc) {
		t.Fatalf("expected nested items after fallback, got %#v", doc)
	}
	stats := MenuStats(doc)
	if stats.ItemCount != 1 {
		t.Fatalf("expected 1 nested item, got %d", stats.ItemCount)
	}
}

func TestGetMenuDocumentReturnsFallbackError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/menus/main":
			_, _ = w.Write([]byte(`{"id":"m1","slug":"main","handle":"main","name":"Main"}`))
		case "/menus":
			http.Error(w, "nested menus unavailable", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, err := GetMenuDocument(context.Background(), New(srv.URL, ""), "main")
	if err == nil {
		t.Fatal("expected nested fallback error")
	}
}
