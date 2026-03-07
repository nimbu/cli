package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertProjectAppAddsAppsBlockWithoutRemovingExistingKeys(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ProjectFileName)
	initial := "site: demo\ntheme: storefront\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	err := UpsertProjectApp(path, AppProjectConfig{
		ID:   "storefront",
		Name: "storefront",
		Dir:  "code",
		Glob: "**/*.js",
		Host: "api.nimbu.io",
		Site: "demo",
	})
	if err != nil {
		t.Fatalf("upsert project app: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	for _, needle := range []string{"site: demo", "theme: storefront", "apps:", "id: storefront", "glob: \"**/*.js\""} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in config:\n%s", needle, text)
		}
	}
}

func TestUpsertProjectAppReplacesExistingHostSiteMatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ProjectFileName)
	initial := "apps:\n  - id: storefront\n    name: old\n    dir: old-code\n    glob: '*.js'\n    host: api.nimbu.io\n    site: demo\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	err := UpsertProjectApp(path, AppProjectConfig{
		ID:   "storefront",
		Name: "new",
		Dir:  "code",
		Glob: "**/*.js",
		Host: "api.nimbu.io",
		Site: "demo",
	})
	if err != nil {
		t.Fatalf("upsert project app: %v", err)
	}

	cfg, err := ReadProjectConfigFrom(path)
	if err != nil {
		t.Fatalf("read project config: %v", err)
	}
	if len(cfg.Apps) != 1 {
		t.Fatalf("app count = %d", len(cfg.Apps))
	}
	if cfg.Apps[0].Name != "new" || cfg.Apps[0].Dir != "code" {
		t.Fatalf("unexpected app: %#v", cfg.Apps[0])
	}
}
