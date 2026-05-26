package themes

import (
	"strings"
	"testing"

	projectconfig "github.com/nimbu/cli/internal/config"
)

func TestResolveConfigDefaults(t *testing.T) {
	root := t.TempDir()
	cfg, err := ResolveConfig(root, projectconfig.ProjectConfig{Theme: "starter"}, "")
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}

	if cfg.Theme != "starter" {
		t.Fatalf("theme mismatch: %q", cfg.Theme)
	}
	if len(cfg.Roots) != 7 {
		t.Fatalf("roots mismatch: got %d", len(cfg.Roots))
	}
	if len(cfg.Generated) != 3 {
		t.Fatalf("generated mismatch: %#v", cfg.Generated)
	}
	wantIgnores := []string{"**/*.map", "**/Thumbs.db", "**/ehthumbs.db", "**/Desktop.ini", "**/*~", "**/*.swp", "**/*.swo", "**/*.tmp", "**/*.temp"}
	if strings.Join(cfg.Ignore, ",") != strings.Join(wantIgnores, ",") {
		t.Fatalf("ignore mismatch: %#v", cfg.Ignore)
	}
}

func TestCollectAllSkipsDefaultIgnoredFiles(t *testing.T) {
	root := t.TempDir()
	cfg, err := ResolveConfig(root, projectconfig.ProjectConfig{Theme: "starter"}, "")
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}

	writeThemeTestFile(t, root, "javascripts/account.js", "console.log('ok')")
	writeThemeTestFile(t, root, "javascripts/account.js.map", "{}")
	writeThemeTestFile(t, root, "javascripts/Thumbs.db", "")
	writeThemeTestFile(t, root, "javascripts/ehthumbs.db", "")
	writeThemeTestFile(t, root, "javascripts/Desktop.ini", "")
	writeThemeTestFile(t, root, "stylesheets/theme.css~", "")
	writeThemeTestFile(t, root, "stylesheets/theme.css.swp", "")
	writeThemeTestFile(t, root, "stylesheets/theme.css.swo", "")
	writeThemeTestFile(t, root, "stylesheets/theme.css.tmp", "")
	writeThemeTestFile(t, root, "stylesheets/theme.css.temp", "")

	resources, err := CollectAll(cfg)
	if err != nil {
		t.Fatalf("collect all: %v", err)
	}

	got := make([]string, len(resources))
	for i, resource := range resources {
		got[i] = resource.LocalPath
	}
	if strings.Join(got, ",") != "javascripts/account.js" {
		t.Fatalf("resources = %#v", got)
	}
}

func TestResolveConfigRejectsNonThemeRoots(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveConfig(root, projectconfig.ProjectConfig{
		Theme: "starter",
		Sync: &projectconfig.SyncConfig{
			Roots: projectconfig.SyncRootsConfig{
				Assets: []string{"code"},
			},
		},
	}, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not a theme resource root") {
		t.Fatalf("unexpected error: %v", err)
	}
}
