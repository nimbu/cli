package themes

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseLiquidDependencies(t *testing.T) {
	code := `
{% include "header" %}
{% include 'cards/card.liquid' with item %}
{% include footer %}
{% include single_person, user: user %}
{% include "icons/icon.arrow.liquid" %}
{% include current_section.snippet %}
{% layout "default.liquid" %}
{% comment %}{% include "ignored" %}{% endcomment %}
{% raw %}{% layout "ignored" %}{% endraw %}
`

	deps, warnings := parseLiquidDependencies(Resource{Kind: KindTemplate, RemoteName: "page.liquid", DisplayPath: "templates/page.liquid"}, []byte(code))

	want := []resourceKey{
		{kind: KindSnippet, remoteName: "cards/card.liquid"},
		{kind: KindLayout, remoteName: "default.liquid"},
		{kind: KindSnippet, remoteName: "footer.liquid"},
		{kind: KindSnippet, remoteName: "header.liquid"},
		{kind: KindSnippet, remoteName: "icons/icon.arrow.liquid"},
		{kind: KindSnippet, remoteName: "single_person.liquid"},
	}
	if !reflect.DeepEqual(deps, want) {
		t.Fatalf("dependencies = %#v, want %#v", deps, want)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "dynamic include") {
		t.Fatalf("warnings = %#v, want one dynamic include warning", warnings)
	}
}

func TestOrderResourceContentByLiquidDependencies(t *testing.T) {
	input := []ResourceContent{
		{
			Resource: Resource{Kind: KindTemplate, RemoteName: "page.liquid", DisplayPath: "templates/page.liquid"},
			Content:  []byte(`{% layout "default.liquid" %}{% include "body" %}`),
		},
		{
			Resource: Resource{Kind: KindLayout, RemoteName: "default.liquid", DisplayPath: "layouts/default.liquid"},
			Content:  []byte(`{% include "head.liquid" %}`),
		},
		{
			Resource: Resource{Kind: KindSnippet, RemoteName: "body.liquid", DisplayPath: "snippets/body.liquid"},
			Content:  []byte(`{% include "atoms/button" %}`),
		},
		{
			Resource: Resource{Kind: KindSnippet, RemoteName: "head.liquid", DisplayPath: "snippets/head.liquid"},
			Content:  []byte(`head`),
		},
		{
			Resource: Resource{Kind: KindSnippet, RemoteName: "atoms/button.liquid", DisplayPath: "snippets/atoms/button.liquid"},
			Content:  []byte(`button`),
		},
		{
			Resource: Resource{Kind: KindAsset, RemoteName: "logo.svg", DisplayPath: "logo.svg"},
			Content:  []byte(`<svg></svg>`),
		},
	}

	ordered, warnings := OrderResourceContentByLiquidDependencies(input)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	got := resourcePaths(ordered)
	want := []string{
		"snippets/atoms/button.liquid",
		"snippets/body.liquid",
		"snippets/head.liquid",
		"layouts/default.liquid",
		"templates/page.liquid",
		"logo.svg",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %#v, want %#v", got, want)
	}
}

func TestOrderResourceContentWarnsForMissingAndCycles(t *testing.T) {
	input := []ResourceContent{
		{
			Resource: Resource{Kind: KindTemplate, RemoteName: "page.liquid", DisplayPath: "templates/page.liquid"},
			Content:  []byte(`{% include "missing" %}`),
		},
		{
			Resource: Resource{Kind: KindSnippet, RemoteName: "a.liquid", DisplayPath: "snippets/a.liquid"},
			Content:  []byte(`{% include "b" %}`),
		},
		{
			Resource: Resource{Kind: KindSnippet, RemoteName: "b.liquid", DisplayPath: "snippets/b.liquid"},
			Content:  []byte(`{% include "a" %}`),
		},
	}

	ordered, warnings := OrderResourceContentByLiquidDependencies(input)
	if got := resourcePaths(ordered); !reflect.DeepEqual(got, []string{"templates/page.liquid", "snippets/a.liquid", "snippets/b.liquid"}) {
		t.Fatalf("order = %#v", got)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %#v, want transfer-set and cycle warnings", warnings)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "dependency not in transfer set") {
		t.Fatalf("warnings missing transfer-set warning: %#v", warnings)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "cycle") {
		t.Fatalf("warnings missing cycle warning: %#v", warnings)
	}
}

func TestExpandTransferSetWithLiquidDependenciesAddsTransitiveSnippets(t *testing.T) {
	page := ResourceContent{
		Resource: Resource{Kind: KindTemplate, RemoteName: "page.liquid", DisplayPath: "templates/page.liquid"},
		Content:  []byte(`{% include 'svg/a' %}`),
	}
	snippetA := ResourceContent{
		Resource: Resource{Kind: KindSnippet, RemoteName: "svg/a.liquid", DisplayPath: "snippets/svg/a.liquid"},
		Content:  []byte(`{% include 'svg/b' %}`),
	}
	snippetB := ResourceContent{
		Resource: Resource{Kind: KindSnippet, RemoteName: "svg/b.liquid", DisplayPath: "snippets/svg/b.liquid"},
		Content:  []byte(`b`),
	}
	unused := ResourceContent{
		Resource: Resource{Kind: KindSnippet, RemoteName: "unused.liquid", DisplayPath: "snippets/unused.liquid"},
		Content:  []byte(`{% include 'svg/a' %}`),
	}

	expanded, added, warnings := ExpandTransferSetWithLiquidDependencies(
		[]ResourceContent{page},
		[]ResourceContent{page, snippetA, snippetB, unused},
	)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	got := resourcePaths(expanded)
	want := []string{"templates/page.liquid", "snippets/svg/a.liquid", "snippets/svg/b.liquid"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded = %#v, want %#v", got, want)
	}
	if !expanded[1].Resource.Dependency || !expanded[2].Resource.Dependency {
		t.Fatalf("added files should be marked as dependencies: %#v", expanded)
	}
	if expanded[0].Resource.Dependency {
		t.Fatal("selected template should not be marked as a dependency")
	}
	if len(added) != 2 {
		t.Fatalf("added = %#v", added)
	}
	if added[0].Path != "snippets/svg/a.liquid" || added[0].ReferencedBy != "templates/page.liquid" {
		t.Fatalf("first added = %#v", added[0])
	}
	if added[1].Path != "snippets/svg/b.liquid" || added[1].ReferencedBy != "snippets/svg/a.liquid" {
		t.Fatalf("second added = %#v", added[1])
	}
}

func TestExpandTransferSetWithLiquidDependenciesWarnsWhenMissingLocally(t *testing.T) {
	page := ResourceContent{
		Resource: Resource{Kind: KindTemplate, RemoteName: "page.liquid", DisplayPath: "templates/page.liquid"},
		Content:  []byte(`{% include 'missing' %}`),
	}

	expanded, added, warnings := ExpandTransferSetWithLiquidDependencies(
		[]ResourceContent{page},
		[]ResourceContent{page},
	)
	if len(added) != 0 {
		t.Fatalf("added = %#v", added)
	}
	if got := resourcePaths(expanded); !reflect.DeepEqual(got, []string{"templates/page.liquid"}) {
		t.Fatalf("expanded = %#v", got)
	}
	if len(warnings) != 1 || warnings[0] != "templates/page.liquid references snippets/missing.liquid, which is not in this theme" {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func resourcePaths(items []ResourceContent) []string {
	paths := make([]string, len(items))
	for i, item := range items {
		paths[i] = item.Resource.DisplayPath
	}
	return paths
}
