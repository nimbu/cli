package channeltypes

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"unicode"
)

// reservedTypeNames are identifiers a generated channel type must not shadow:
// the imported SDK helpers, names the cloud typings declare, and the TypeScript
// globals people reach for in hand-written code next to the generated file.
// The NimbuChannels augmentation does not depend on this list: it goes through
// collision-proof aliases (see aliasName).
var reservedTypeNames = map[string]bool{
	tsReferenceTo: true, tsReferenceMany: true, tsSelect: true, tsMultiSelect: true,
	tsDateTime: true, tsISODate: true, tsJSONField: true, tsNimbuFile: true,
	tsNimbuGallery: true, tsNimbuCustomer: true,
	"ChannelConfig": true, "NimbuChannels": true, "NimbuObject": true, "Nimbu": true,
	"Array": true, "Boolean": true, "Date": true, "Error": true, "Exclude": true,
	"Extract": true, "Function": true, "JSON": true, "Map": true, "Math": true,
	"NonNullable": true, "Number": true, "Object": true, "Omit": true, "Partial": true,
	"Pick": true, "Promise": true, "Readonly": true, "Record": true, "RegExp": true,
	"Required": true, "ReturnType": true, "Set": true, "String": true, "Symbol": true,
}

// aliasPrefix marks the local aliases the NimbuChannels augmentation refers to.
// Inside `declare module` TypeScript resolves names against the augmented
// module's exports first, so a channel type named like a cloud export (Site,
// CallbackEvent, ...) would silently resolve to the SDK type. No SDK export
// starts with this prefix.
const aliasPrefix = "__NimbuChannel_"

func aliasName(typeName string) string {
	return aliasPrefix + typeName
}

// assignTypeNames maps slugs to unique, valid TypeScript type names.
//
// A slug gets the PascalCase of its words unless another slug in the set has
// the same PascalCase (blog-posts and blog_posts). In that case the slug that
// sorts first keeps the plain name and each other one gets a name derived from
// its own spelling: underscores kept (Blog_Posts), or a hash of the slug when
// that still collides. Loser names therefore do not depend on how many other
// colliding channels exist or in which order they were added.
func assignTypeNames(slugs []string) map[string]string {
	unique := map[string]bool{}
	for _, slug := range slugs {
		unique[slug] = true
	}
	sorted := make([]string, 0, len(unique))
	for slug := range unique {
		sorted = append(sorted, slug)
	}
	sort.Strings(sorted)

	names := make(map[string]string, len(sorted))
	used := map[string]bool{}
	var losers []string
	for _, slug := range sorted {
		base := baseTypeName(slug)
		if used[base] {
			losers = append(losers, slug)
			continue
		}
		used[base] = true
		names[slug] = base
	}
	// Plain names are all taken before any loser picks one, so a loser can
	// never steal the plain name of a slug that sorts after it.
	for _, slug := range losers {
		name := literalTypeName(slug)
		if used[name] {
			name = fmt.Sprintf("%s_%08x", baseTypeName(slug), slugHash(slug))
		}
		for i := 2; used[name]; i++ {
			name = fmt.Sprintf("%s_%08x_%d", baseTypeName(slug), slugHash(slug), i)
		}
		used[name] = true
		names[slug] = name
	}
	return names
}

func baseTypeName(slug string) string {
	name := pascalCase(slug, false)
	if reservedTypeNames[name] {
		name += "Channel"
	}
	return name
}

// literalTypeName is the PascalCase name with the slug's underscores kept.
func literalTypeName(slug string) string {
	name := pascalCase(slug, true)
	if reservedTypeNames[name] {
		name += "Channel"
	}
	return name
}

func slugHash(slug string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(slug))
	return h.Sum32()
}

// pascalCase turns a slug into an identifier: every run of letters and digits
// becomes a word with an upper-cased first rune; anything else separates words.
// With keepUnderscores, words split on '_' are joined with '_'.
func pascalCase(slug string, keepUnderscores bool) string {
	var b strings.Builder
	upperNext := true
	for _, r := range slug {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if upperNext {
				r = unicode.ToUpper(r)
				upperNext = false
			}
			b.WriteRune(r)
		case r == '_' && keepUnderscores:
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "_") {
				b.WriteRune('_')
			}
			upperNext = true
		default:
			upperNext = true
		}
	}
	name := strings.TrimSuffix(b.String(), "_")
	if name == "" {
		return "Channel"
	}
	if first := []rune(name)[0]; unicode.IsDigit(first) {
		return "Channel" + name
	}
	return name
}
