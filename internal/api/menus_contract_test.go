package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
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

func TestNormalizeMenuDocumentForWriteStripsTargetPageAndFillsAliases(t *testing.T) {
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
			map[string]any{
				"name":       "Already",
				"target_url": "/already",
				"title":      "IgnoredTitle",
				"url":        "/ignored",
			},
		},
	}

	NormalizeMenuDocumentForWrite(doc)

	root := doc["items"].([]any)[0].(map[string]any)
	if _, ok := root["target_page"]; ok {
		t.Fatal("expected target_page to be removed from root item")
	}
	if root["url"] != "/" {
		t.Fatalf("expected url alias preserved, got %#v", root["url"])
	}
	if root["name"] != "Home" {
		t.Fatalf("expected title copied to name, got %#v", root["name"])
	}
	if root["target_url"] != "/" {
		t.Fatalf("expected url copied to target_url, got %#v", root["target_url"])
	}
	child := root["children"].([]any)[0].(map[string]any)
	if _, ok := child["target_page"]; ok {
		t.Fatal("expected target_page to be removed from child item")
	}
	if child["name"] != "Wine" || child["target_url"] != "/wine" {
		t.Fatalf("expected child aliases filled, got %#v", child)
	}

	named := doc["items"].([]any)[1].(map[string]any)
	if named["name"] != "Already" || named["target_url"] != "/already" {
		t.Fatalf("expected existing name/target_url preserved, got %#v", named)
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

func TestGetMenuDocumentRecoversNestedTreeFromFlatSingular(t *testing.T) {
	var listCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/menus/main":
			// Flat projection: siblings with depth, no nested children arrays.
			_, _ = w.Write([]byte(`{
				"id":"m1","slug":"main","handle":"main","name":"Main",
				"items":[
					{"name":"Home","depth":0,"children":[]},
					{"name":"Wine","depth":1,"children":[]},
					{"name":"Boxes","depth":1,"children":[]}
				]
			}`))
		case "/menus":
			listCalls.Add(1)
			if got := r.URL.Query().Get("nested"); got != "1" {
				t.Fatalf("expected nested=1 query, got %q", got)
			}
			_, _ = w.Write([]byte(`[{
				"id":"m1","slug":"main","handle":"main","name":"Main",
				"items":[
					{"name":"Home","children":[]},
					{"name":"Shop","children":[
						{"name":"Wine","children":[]},
						{"name":"Boxes","children":[]}
					]}
				]
			}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	doc, err := GetMenuDocument(context.Background(), New(srv.URL, ""), "main")
	if err != nil {
		t.Fatalf("get menu document: %v", err)
	}
	if listCalls.Load() != 1 {
		t.Fatalf("expected nested list fallback, got %d calls", listCalls.Load())
	}
	stats := MenuStats(doc)
	if stats.MaxDepth != 2 {
		t.Fatalf("expected nested max depth 2, got %d (%#v)", stats.MaxDepth, doc["items"])
	}
	items := doc["items"].([]any)
	shop := items[1].(map[string]any)
	children, ok := shop["children"].([]any)
	if !ok || len(children) != 2 {
		t.Fatalf("expected Shop children from nested fallback, got %#v", shop["children"])
	}
}

func TestGetMenuDocumentKeepsSingularWhenAlreadyNested(t *testing.T) {
	var listCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/menus/main":
			_, _ = w.Write([]byte(`{
				"id":"m1","slug":"main","handle":"main","name":"Main",
				"items":[{"name":"Shop","children":[{"name":"Wine"}]}]
			}`))
		case "/menus":
			listCalls.Add(1)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	doc, err := GetMenuDocument(context.Background(), New(srv.URL, ""), "main")
	if err != nil {
		t.Fatalf("get menu document: %v", err)
	}
	if listCalls.Load() != 0 {
		t.Fatalf("expected no nested list call for already-nested singular, got %d", listCalls.Load())
	}
	if MenuStats(doc).MaxDepth != 2 {
		t.Fatalf("expected depth 2, got %d", MenuStats(doc).MaxDepth)
	}
}

func TestGetMenuDocumentKeepsFlatSingularWhenNestedFallbackFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/menus/main":
			_, _ = w.Write([]byte(`{
				"id":"m1","slug":"main","handle":"main","name":"Main",
				"items":[{"name":"Home","depth":0}]
			}`))
		case "/menus":
			http.Error(w, "nested menus unavailable", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	doc, err := GetMenuDocument(context.Background(), New(srv.URL, ""), "main")
	if err != nil {
		t.Fatalf("expected singular doc retained when nested fails, got %v", err)
	}
	if !MenuDocumentHasItems(doc) {
		t.Fatalf("expected flat singular items retained, got %#v", doc)
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

func TestMenuNestingLost(t *testing.T) {
	if MenuNestingLost(MenuDocumentStats{MaxDepth: 1}, MenuDocumentStats{MaxDepth: 1}) {
		t.Fatal("flat submit should not report nesting lost")
	}
	if !MenuNestingLost(MenuDocumentStats{MaxDepth: 3}, MenuDocumentStats{MaxDepth: 1}) {
		t.Fatal("expected nesting lost when depth drops")
	}
	if MenuNestingLost(MenuDocumentStats{MaxDepth: 3}, MenuDocumentStats{MaxDepth: 3}) {
		t.Fatal("equal depth should not report nesting lost")
	}
	if !MenuNestingLost(
		MenuDocumentStats{ItemCount: 4, MaxDepth: 2, Shape: `["Shop",["Wine","Gifts"]]`},
		MenuDocumentStats{ItemCount: 3, MaxDepth: 2, Shape: `["Shop",["Wine"]]`},
	) {
		t.Fatal("expected a missing sibling to report structure loss")
	}
	if !MenuNestingLost(
		MenuDocumentStats{HasItems: true, ItemCount: 2, MaxDepth: 1, Shape: `["Home","Shop"]`},
		MenuDocumentStats{HasItems: true, ItemCount: 1, MaxDepth: 1, Shape: `["Home"]`},
	) {
		t.Fatal("expected a missing flat item to report structure loss")
	}
}

func TestReconcileMenuDocumentAppendsTombstonesDeterministically(t *testing.T) {
	current := MenuDocument{"items": []any{
		map[string]any{"id": "z"},
		map[string]any{"id": "a"},
		map[string]any{"id": "m"},
	}}
	desired := MenuDocument{"items": []any{}}

	ReconcileMenuDocument(current, desired)

	items := desired["items"].([]any)
	got := make([]string, 0, len(items))
	for _, raw := range items {
		got = append(got, raw.(map[string]any)["id"].(string))
	}
	want := []string{"a", "m", "z"}
	if !slices.Equal(got, want) {
		t.Fatalf("tombstone order = %v, want %v", got, want)
	}
}

func TestPatchMenuDocumentReplaceOptional(t *testing.T) {
	var gotReplace string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReplace = r.URL.Query().Get("replace")
		_, _ = w.Write([]byte(`{"id":"m1","slug":"main","name":"Main"}`))
	}))
	defer srv.Close()

	client := New(srv.URL, "")
	if _, err := PatchMenuDocument(context.Background(), client, "main", MenuDocument{"name": "Main"}); err != nil {
		t.Fatalf("patch without replace: %v", err)
	}
	if gotReplace != "" {
		t.Fatalf("expected no replace by default, got %q", gotReplace)
	}

	if _, err := PatchMenuDocument(context.Background(), client, "main", MenuDocument{"name": "Main"}, WithReplace(true)); err != nil {
		t.Fatalf("patch with replace: %v", err)
	}
	if gotReplace != "1" {
		t.Fatalf("expected replace=1, got %q", gotReplace)
	}
}
