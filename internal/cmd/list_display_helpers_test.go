package cmd

import (
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/themes"
)

func TestEntryDisplayTitle(t *testing.T) {
	t.Run("title preferred", func(t *testing.T) {
		got := entryDisplayTitle(api.Entry{ID: "1", Slug: "entry-1", Title: "Hello"})
		if got != "Hello" {
			t.Fatalf("expected title, got %q", got)
		}
	})

	t.Run("fields title fallback", func(t *testing.T) {
		got := entryDisplayTitle(api.Entry{ID: "1", Slug: "entry-1", Fields: map[string]any{"title": "From Fields"}})
		if got != "From Fields" {
			t.Fatalf("expected fields title, got %q", got)
		}
	})

	t.Run("title field value fallback", func(t *testing.T) {
		got := entryDisplayTitle(api.Entry{ID: "1", Slug: "entry-1", Extra: map[string]any{"title_field_value": "Project Naam"}})
		if got != "Project Naam" {
			t.Fatalf("expected title_field_value, got %q", got)
		}
	})

	t.Run("slug fallback", func(t *testing.T) {
		got := entryDisplayTitle(api.Entry{ID: "1", Slug: "entry-1", Fields: map[string]any{"title": 123}})
		if got != "entry-1" {
			t.Fatalf("expected slug fallback, got %q", got)
		}
	})

	t.Run("id fallback", func(t *testing.T) {
		got := entryDisplayTitle(api.Entry{ID: "id-only"})
		if got != "id-only" {
			t.Fatalf("expected id fallback, got %q", got)
		}
	})
}

func TestBlogDisplayHandle(t *testing.T) {
	if got := blogDisplayHandle(api.Blog{ID: "blog-id", Handle: "news"}); got != "news" {
		t.Fatalf("expected handle, got %q", got)
	}
	if got := blogDisplayHandle(api.Blog{ID: "blog-id"}); got != "blog-id" {
		t.Fatalf("expected id fallback, got %q", got)
	}
}

func TestOrderDisplayNumber(t *testing.T) {
	if got := orderDisplayNumber(api.Order{ID: "1234567890", Number: "A-42"}); got != "A-42" {
		t.Fatalf("expected number, got %q", got)
	}
	if got := orderDisplayNumber(api.Order{ID: "1234567890"}); got != "12345678" {
		t.Fatalf("expected short id fallback, got %q", got)
	}
	if got := orderDisplayNumber(api.Order{ID: "1234"}); got != "1234" {
		t.Fatalf("expected full id fallback, got %q", got)
	}
}

func TestThemeResourcePath(t *testing.T) {
	if got := themeResourcePath(themes.KindAsset, api.ThemeResource{Path: "/css/app.css", Name: "ignored"}); got != "css/app.css" {
		t.Fatalf("expected path, got %q", got)
	}
	if got := themeResourcePath(themes.KindSnippet, api.ThemeResource{Name: "header.liquid"}); got != "snippets/header.liquid" {
		t.Fatalf("expected snippet path, got %q", got)
	}
	if got := themeResourcePath(themes.KindLayout, api.ThemeResource{Name: "layout.liquid"}); got != "layouts/layout.liquid" {
		t.Fatalf("expected layout path, got %q", got)
	}
	if got := themeResourcePath(themes.KindTemplate, api.ThemeResource{ID: "abc"}); got != "templates/abc" {
		t.Fatalf("expected id fallback, got %q", got)
	}
}

func TestPaginateThemeFileListRows(t *testing.T) {
	rows := []themeFileListItem{
		{Path: "a"}, {Path: "b"}, {Path: "c"}, {Path: "d"},
	}

	page1 := paginateThemeFileListRows(rows, 1, 2)
	if len(page1) != 2 || page1[0].Path != "a" || page1[1].Path != "b" {
		t.Fatalf("unexpected page1: %#v", page1)
	}

	page2 := paginateThemeFileListRows(rows, 2, 2)
	if len(page2) != 2 || page2[0].Path != "c" || page2[1].Path != "d" {
		t.Fatalf("unexpected page2: %#v", page2)
	}

	page3 := paginateThemeFileListRows(rows, 3, 2)
	if len(page3) != 0 {
		t.Fatalf("expected empty page, got %#v", page3)
	}
}
