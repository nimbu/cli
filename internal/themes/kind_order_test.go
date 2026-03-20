package themes

import (
	"testing"
)

func TestGroupByKind(t *testing.T) {
	tests := []struct {
		name  string
		input []Resource
		want  map[Kind]int // expected count per kind
	}{
		{
			name:  "empty",
			input: nil,
			want:  map[Kind]int{},
		},
		{
			name: "single kind",
			input: []Resource{
				{Kind: KindLayout, DisplayPath: "layouts/a.liquid"},
				{Kind: KindLayout, DisplayPath: "layouts/b.liquid"},
			},
			want: map[Kind]int{KindLayout: 2},
		},
		{
			name: "mixed kinds",
			input: []Resource{
				{Kind: KindTemplate, DisplayPath: "templates/page.liquid"},
				{Kind: KindLayout, DisplayPath: "layouts/default.liquid"},
				{Kind: KindSnippet, DisplayPath: "snippets/nav.liquid"},
				{Kind: KindAsset, DisplayPath: "assets/logo.png"},
				{Kind: KindTemplate, DisplayPath: "templates/blog.liquid"},
			},
			want: map[Kind]int{
				KindLayout:   1,
				KindSnippet:  1,
				KindTemplate: 2,
				KindAsset:    1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GroupByKind(tt.input)
			for k, wantN := range tt.want {
				if len(got[k]) != wantN {
					t.Errorf("GroupByKind()[%s] = %d resources, want %d", k, len(got[k]), wantN)
				}
			}
			// Verify no unexpected kinds
			for k, v := range got {
				if _, ok := tt.want[k]; !ok {
					t.Errorf("GroupByKind() has unexpected kind %s with %d resources", k, len(v))
				}
			}
		})
	}
}

func TestSortByKindOrder(t *testing.T) {
	input := []Resource{
		{Kind: KindAsset, DisplayPath: "assets/z.png"},
		{Kind: KindTemplate, DisplayPath: "templates/page.liquid"},
		{Kind: KindLayout, DisplayPath: "layouts/default.liquid"},
		{Kind: KindSnippet, DisplayPath: "snippets/nav.liquid"},
		{Kind: KindAsset, DisplayPath: "assets/a.png"},
		{Kind: KindTemplate, DisplayPath: "templates/blog.liquid"},
		{Kind: KindLayout, DisplayPath: "layouts/blank.liquid"},
		{Kind: KindSnippet, DisplayPath: "snippets/footer.liquid"},
	}

	got := SortByKindOrder(input)

	// Verify input is not mutated
	if input[0].Kind != KindAsset || input[0].DisplayPath != "assets/z.png" {
		t.Fatal("SortByKindOrder mutated the input slice")
	}

	// Expected order: layouts (alpha), snippets (alpha), templates (alpha), assets (alpha)
	expected := []struct {
		kind Kind
		path string
	}{
		{KindLayout, "layouts/blank.liquid"},
		{KindLayout, "layouts/default.liquid"},
		{KindSnippet, "snippets/footer.liquid"},
		{KindSnippet, "snippets/nav.liquid"},
		{KindTemplate, "templates/blog.liquid"},
		{KindTemplate, "templates/page.liquid"},
		{KindAsset, "assets/a.png"},
		{KindAsset, "assets/z.png"},
	}

	if len(got) != len(expected) {
		t.Fatalf("SortByKindOrder() returned %d items, want %d", len(got), len(expected))
	}

	for i, want := range expected {
		if got[i].Kind != want.kind || got[i].DisplayPath != want.path {
			t.Errorf("SortByKindOrder()[%d] = {%s, %s}, want {%s, %s}",
				i, got[i].Kind, got[i].DisplayPath, want.kind, want.path)
		}
	}
}

func TestSortByKindOrder_Empty(t *testing.T) {
	got := SortByKindOrder(nil)
	if len(got) != 0 {
		t.Errorf("SortByKindOrder(nil) = %d items, want 0", len(got))
	}
}
