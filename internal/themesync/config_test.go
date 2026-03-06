package themesync

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
