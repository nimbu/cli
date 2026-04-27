package themes

import "sort"

// KindOrder defines fallback transfer order when no more specific ordering is
// required.
var KindOrder = []Kind{KindSnippet, KindLayout, KindTemplate, KindAsset}

var kindRank = func() map[Kind]int {
	m := make(map[Kind]int, len(KindOrder))
	for i, k := range KindOrder {
		m[k] = i
	}
	return m
}()

// GroupByKind groups resources by their Kind.
func GroupByKind(resources []Resource) map[Kind][]Resource {
	grouped := make(map[Kind][]Resource)
	for _, r := range resources {
		grouped[r.Kind] = append(grouped[r.Kind], r)
	}
	return grouped
}

// SortByKindOrder returns a new slice sorted by KindOrder, then alphabetically
// by DisplayPath within each kind.
func SortByKindOrder(resources []Resource) []Resource {
	sorted := make([]Resource, len(resources))
	copy(sorted, resources)
	sort.SliceStable(sorted, func(i, j int) bool {
		return resourceLess(sorted[i], sorted[j])
	})
	return sorted
}

func resourceLess(a, b Resource) bool {
	ar, br := kindRank[a.Kind], kindRank[b.Kind]
	if ar != br {
		return ar < br
	}
	if a.DisplayPath == b.DisplayPath {
		return a.Kind < b.Kind
	}
	return a.DisplayPath < b.DisplayPath
}
