package cmd

import (
	"slices"
	"strings"
	"testing"
)

func TestProjectRelationIDs(t *testing.T) {
	t.Parallel()

	current := []string{"a", "b", "c"}
	tests := []struct {
		name string
		raw  any
		want []string
	}{
		{
			name: "array replace",
			raw:  []any{"x"},
			want: []string{"x"},
		},
		{
			name: "add relation",
			raw: map[string]any{
				"__op": "AddRelation",
				"objects": []any{
					map[string]any{"__type": "Reference", "id": "d"},
				},
			},
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "remove relation",
			raw: map[string]any{
				"__op":    "RemoveRelation",
				"objects": []any{map[string]any{"id": "b"}},
			},
			want: []string{"a", "c"},
		},
		{
			name: "add reference alias",
			raw: map[string]any{
				"__op":    "AddReference",
				"objects": []any{map[string]any{"id": "d"}},
			},
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "explicit null clears",
			raw:  nil,
			want: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, projected, err := projectRelationIDs(current, tc.raw)
			if err != nil {
				t.Fatalf("project: %v", err)
			}
			if !projected {
				t.Fatal("expected projection")
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestRelationShrinksOverHalf(t *testing.T) {
	t.Parallel()

	if relationShrinksOverHalf(4, 2) {
		t.Fatal("exactly half should be allowed")
	}
	if !relationShrinksOverHalf(4, 1) {
		t.Fatal("4 → 1 should refuse")
	}
	if relationShrinksOverHalf(2, 1) {
		t.Fatal("2 → 1 is half, not more")
	}
	if relationShrinksOverHalf(0, 0) {
		t.Fatal("empty should not refuse")
	}
}

func TestProjectRelationIDsRejectsScalar(t *testing.T) {
	t.Parallel()

	for _, raw := range []any{"c1", 1.0, true} {
		_, _, err := projectRelationIDs([]string{"a", "b"}, raw)
		if err == nil || !strings.Contains(err.Error(), "expected an array of IDs or an __op object") {
			t.Fatalf("raw %#v: expected scalar error, got %v", raw, err)
		}
	}
}

func TestRequireRelationShrinksNilFlagsRefuses(t *testing.T) {
	t.Parallel()

	err := requireRelationShrinks(nil, "bingo", []roleRelationDiff{{
		Field: "customers", Before: 4, After: 1, Projected: true, BeforeKnown: true,
	}})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force error, got %v", err)
	}
}

func TestProjectRelationIDsUnknownOpPassesThrough(t *testing.T) {
	t.Parallel()

	got, projected, err := projectRelationIDs([]string{"a"}, map[string]any{
		"__op":    "AddUnique",
		"objects": []any{map[string]any{"id": "b"}},
	})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if projected {
		t.Fatalf("unknown op should not project, got %#v", got)
	}
}
