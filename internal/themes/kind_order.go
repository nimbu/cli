package themes

import "sort"

// KindOrder defines dependency-based upload order:
// layouts first (templates reference them), then snippets (templates include them),
// then templates, then assets.
var KindOrder = []Kind{KindLayout, KindSnippet, KindTemplate, KindAsset}

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

// SortByKindOrder returns a new slice sorted in dependency order (KindOrder),
// then alphabetically by DisplayPath within each kind.
func SortByKindOrder(resources []Resource) []Resource {
	sorted := make([]Resource, len(resources))
	copy(sorted, resources)
	sort.SliceStable(sorted, func(i, j int) bool {
		ri, rj := kindRank[sorted[i].Kind], kindRank[sorted[j].Kind]
		if ri != rj {
			return ri < rj
		}
		return sorted[i].DisplayPath < sorted[j].DisplayPath
	})
	return sorted
}
