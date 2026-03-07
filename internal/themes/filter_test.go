package themes

import "testing"

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
