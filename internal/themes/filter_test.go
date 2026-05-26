package themes

import (
	"strings"
	"testing"
)

func TestNormalizeOnlyPathRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	cfg := Config{ProjectRoot: root}

	if _, err := explicitOnlySet(cfg, []string{"../secret"}); err == nil {
		t.Fatal("expected root escape error")
	}
	if _, err := explicitOnlySet(cfg, []string{"/tmp/secret"}); err == nil {
		t.Fatal("expected absolute path error")
	}
}

func TestExplicitOnlySetRejectsUnmanagedPath(t *testing.T) {
	cfg := Config{
		ProjectRoot: t.TempDir(),
		Roots: []RootSpec{
			{Kind: KindSnippet, LocalPath: "snippets"},
		},
	}

	if _, err := explicitOnlySet(cfg, []string{"stylesheets/theme.css"}); err == nil {
		t.Fatal("expected unmanaged path error")
	}
}

func TestCompileSelectionFilterMatchesCategories(t *testing.T) {
	cfg := Config{ProjectRoot: t.TempDir(), Roots: []RootSpec{
		{Kind: KindSnippet, LocalPath: "snippets"},
		{Kind: KindAsset, LocalPath: "stylesheets", RemoteBase: "stylesheets"},
		{Kind: KindAsset, LocalPath: "images", RemoteBase: "images"},
	}}

	filter, err := compileSelectionFilter(cfg, Options{CSSOnly: true})
	if err != nil {
		t.Fatalf("compile filter: %v", err)
	}

	if !filter.Match(Resource{Kind: KindAsset, LocalPath: "stylesheets/theme.css", DisplayPath: "stylesheets/theme.css"}) {
		t.Fatal("expected css file selected")
	}
	if filter.Match(Resource{Kind: KindAsset, LocalPath: "images/logo.png", DisplayPath: "images/logo.png"}) {
		t.Fatal("did not expect image selected by css filter")
	}
	if filter.Match(Resource{Kind: KindSnippet, LocalPath: "snippets/header.liquid", DisplayPath: "snippets/header.liquid"}) {
		t.Fatal("did not expect liquid file selected by css filter")
	}
}

func TestFilterResourcesNoImagesExcludesImageRoot(t *testing.T) {
	cfg := Config{ProjectRoot: t.TempDir(), Roots: []RootSpec{
		{Kind: KindLayout, LocalPath: "layouts"},
		{Kind: KindSnippet, LocalPath: "snippets"},
		{Kind: KindAsset, LocalPath: "stylesheets", RemoteBase: "stylesheets"},
		{Kind: KindAsset, LocalPath: "javascripts", RemoteBase: "javascripts"},
		{Kind: KindAsset, LocalPath: "fonts", RemoteBase: "fonts"},
		{Kind: KindAsset, LocalPath: "images", RemoteBase: "images"},
	}}
	resources := []Resource{
		{Kind: KindLayout, LocalPath: "layouts/default.liquid", DisplayPath: "layouts/default.liquid"},
		{Kind: KindSnippet, LocalPath: "snippets/header.liquid", DisplayPath: "snippets/header.liquid"},
		{Kind: KindAsset, LocalPath: "stylesheets/theme.css", DisplayPath: "stylesheets/theme.css"},
		{Kind: KindAsset, LocalPath: "stylesheets/background.png", DisplayPath: "stylesheets/background.png"},
		{Kind: KindAsset, LocalPath: "javascripts/app.js", DisplayPath: "javascripts/app.js"},
		{Kind: KindAsset, LocalPath: "fonts/app.woff2", DisplayPath: "fonts/app.woff2"},
		{Kind: KindAsset, LocalPath: "images/logo.svg", DisplayPath: "images/logo.svg"},
	}

	filtered, err := FilterResources(cfg, resources, Options{NoImages: true})
	if err != nil {
		t.Fatalf("filter resources: %v", err)
	}

	got := make([]string, len(filtered))
	for i, resource := range filtered {
		got[i] = resource.DisplayPath
	}
	want := []string{
		"layouts/default.liquid",
		"snippets/header.liquid",
		"stylesheets/theme.css",
		"stylesheets/background.png",
		"javascripts/app.js",
		"fonts/app.woff2",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("filtered = %#v, want %#v", got, want)
	}
}

func TestFilterResourcesNoImagesExcludesRemoteImageResources(t *testing.T) {
	cfg := Config{ProjectRoot: t.TempDir(), Roots: []RootSpec{
		{Kind: KindAsset, LocalPath: "images", RemoteBase: "images"},
		{Kind: KindAsset, LocalPath: "fonts", RemoteBase: "fonts"},
	}}
	resources := []Resource{
		{Kind: KindAsset, RemoteName: "images/logo.svg", DisplayPath: "images/logo.svg"},
		{Kind: KindAsset, RemoteName: "fonts/app.woff2", DisplayPath: "fonts/app.woff2"},
	}

	filtered, err := FilterResources(cfg, resources, Options{NoImages: true})
	if err != nil {
		t.Fatalf("filter resources: %v", err)
	}
	if len(filtered) != 1 || filtered[0].DisplayPath != "fonts/app.woff2" {
		t.Fatalf("filtered = %#v, want only font", filtered)
	}
}

func TestFilterResourcesImagesOnlyWithNoImagesSelectsNothing(t *testing.T) {
	cfg := Config{ProjectRoot: t.TempDir(), Roots: []RootSpec{
		{Kind: KindAsset, LocalPath: "images", RemoteBase: "images"},
		{Kind: KindAsset, LocalPath: "stylesheets", RemoteBase: "stylesheets"},
	}}
	resources := []Resource{
		{Kind: KindAsset, LocalPath: "images/logo.svg", DisplayPath: "images/logo.svg"},
		{Kind: KindAsset, LocalPath: "stylesheets/theme.css", DisplayPath: "stylesheets/theme.css"},
	}

	filtered, err := FilterResources(cfg, resources, Options{ImagesOnly: true, NoImages: true})
	if err != nil {
		t.Fatalf("filter resources: %v", err)
	}
	if len(filtered) != 0 {
		t.Fatalf("filtered = %#v, want none", filtered)
	}
}

func TestCompileSelectionFilterMatchesLiquidOnly(t *testing.T) {
	cfg := Config{ProjectRoot: t.TempDir(), Roots: []RootSpec{{Kind: KindSnippet, LocalPath: "snippets"}}}
	filter, err := compileSelectionFilter(cfg, Options{LiquidOnly: true})
	if err != nil {
		t.Fatalf("compile filter: %v", err)
	}

	if !filter.Match(Resource{Kind: KindSnippet, LocalPath: "snippets/header.liquid", DisplayPath: "snippets/header.liquid"}) {
		t.Fatal("expected snippet selected")
	}
	if filter.Match(Resource{Kind: KindAsset, LocalPath: "images/logo.png", DisplayPath: "images/logo.png"}) {
		t.Fatal("did not expect asset selected")
	}
}

func TestCompileSelectionFilterMatchesOnlySelectors(t *testing.T) {
	cfg := Config{ProjectRoot: t.TempDir(), Roots: []RootSpec{
		{Kind: KindLayout, LocalPath: "layouts"},
		{Kind: KindSnippet, LocalPath: "snippets"},
		{Kind: KindAsset, LocalPath: "stylesheets", RemoteBase: "stylesheets"},
	}}

	filter, err := compileSelectionFilter(cfg, Options{Only: []string{
		"stylesheets/*.css",
		"snippets/",
		"layouts/default.liquid",
	}})
	if err != nil {
		t.Fatalf("compile filter: %v", err)
	}

	if !filter.Match(Resource{Kind: KindAsset, LocalPath: "stylesheets/theme.css", DisplayPath: "stylesheets/theme.css"}) {
		t.Fatal("expected css glob selector to match")
	}
	if filter.Match(Resource{Kind: KindAsset, LocalPath: "stylesheets/theme.js", DisplayPath: "stylesheets/theme.js"}) {
		t.Fatal("did not expect css glob selector to match js")
	}
	if !filter.Match(Resource{Kind: KindSnippet, LocalPath: "snippets/header.liquid", DisplayPath: "snippets/header.liquid"}) {
		t.Fatal("expected directory selector to match nested snippet")
	}
	if !filter.Match(Resource{Kind: KindLayout, LocalPath: "layouts/default.liquid", DisplayPath: "layouts/default.liquid"}) {
		t.Fatal("expected exact selector to match layout")
	}
}

func TestCompileSelectionFilterMatchesDirectorySelectorWithoutSlash(t *testing.T) {
	cfg := Config{ProjectRoot: t.TempDir(), Roots: []RootSpec{
		{Kind: KindSnippet, LocalPath: "snippets"},
		{Kind: KindAsset, LocalPath: "stylesheets", RemoteBase: "stylesheets"},
	}}

	filter, err := compileSelectionFilter(cfg, Options{Only: []string{"snippets"}})
	if err != nil {
		t.Fatalf("compile filter: %v", err)
	}

	if !filter.Match(Resource{Kind: KindSnippet, LocalPath: "snippets/header.liquid", DisplayPath: "snippets/header.liquid"}) {
		t.Fatal("expected directory selector without slash to match nested snippet")
	}
	if filter.Match(Resource{Kind: KindAsset, LocalPath: "stylesheets/theme.css", DisplayPath: "stylesheets/theme.css"}) {
		t.Fatal("did not expect directory selector to match unrelated file")
	}
}

func TestCompileSelectionFilterCombinesCategoriesAndOnlySelectorsAsUnion(t *testing.T) {
	cfg := Config{ProjectRoot: t.TempDir(), Roots: []RootSpec{
		{Kind: KindLayout, LocalPath: "layouts"},
		{Kind: KindAsset, LocalPath: "stylesheets", RemoteBase: "stylesheets"},
		{Kind: KindAsset, LocalPath: "javascript", RemoteBase: "javascript"},
	}}

	filter, err := compileSelectionFilter(cfg, Options{
		CSSOnly: true,
		JSOnly:  true,
		Only:    []string{"layouts/default.liquid"},
	})
	if err != nil {
		t.Fatalf("compile filter: %v", err)
	}

	if !filter.Match(Resource{Kind: KindAsset, LocalPath: "stylesheets/theme.css", DisplayPath: "stylesheets/theme.css"}) {
		t.Fatal("expected css category to match")
	}
	if !filter.Match(Resource{Kind: KindAsset, LocalPath: "javascript/app.js", DisplayPath: "javascript/app.js"}) {
		t.Fatal("expected js category to match")
	}
	if !filter.Match(Resource{Kind: KindLayout, LocalPath: "layouts/default.liquid", DisplayPath: "layouts/default.liquid"}) {
		t.Fatal("expected only layout selector to match with category flags")
	}
}

func TestFilterResourcesRejectsUnmatchedOnlySelector(t *testing.T) {
	cfg := Config{ProjectRoot: t.TempDir(), Roots: []RootSpec{
		{Kind: KindAsset, LocalPath: "javascripts", RemoteBase: "javascripts"},
	}}
	resources := []Resource{
		{Kind: KindAsset, LocalPath: "javascripts/app.js", DisplayPath: "javascripts/app.js"},
	}

	_, err := FilterResources(cfg, resources, Options{Only: []string{"javascript/*.js"}})
	if err == nil {
		t.Fatal("expected unmatched only selector error")
	}
}

func TestOnlyFlagAcceptsGlobSelector(t *testing.T) {
	cfg := Config{ProjectRoot: t.TempDir(), Roots: []RootSpec{
		{Kind: KindAsset, LocalPath: "stylesheets", RemoteBase: "stylesheets"},
		{Kind: KindAsset, LocalPath: "images", RemoteBase: "images"},
	}}
	resources := []Resource{
		{Kind: KindAsset, LocalPath: "stylesheets/theme.css", DisplayPath: "stylesheets/theme.css"},
		{Kind: KindAsset, LocalPath: "images/logo.svg", DisplayPath: "images/logo.svg"},
	}

	filtered, err := FilterResources(cfg, resources, Options{Only: []string{"stylesheets/*.css"}})
	if err != nil {
		t.Fatalf("filter resources: %v", err)
	}
	if len(filtered) != 1 || filtered[0].LocalPath != "stylesheets/theme.css" {
		t.Fatalf("filtered = %#v, want only stylesheet", filtered)
	}
}

func TestCompileSelectionFilterRejectsInvalidSelector(t *testing.T) {
	cfg := Config{ProjectRoot: t.TempDir()}

	if _, err := compileSelectionFilter(cfg, Options{Only: []string{"../theme.css"}}); err == nil {
		t.Fatal("expected only selector escape error")
	}
	if _, err := compileSelectionFilter(cfg, Options{Only: []string{"/tmp/theme.css"}}); err == nil {
		t.Fatal("expected absolute only selector error")
	}
	if _, err := compileSelectionFilter(cfg, Options{Only: []string{`C:\tmp\theme.css`}}); err == nil {
		t.Fatal("expected Windows drive only selector error")
	}
}
