package apps

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nimbu/cli/internal/config"
)

func TestVisibleAppsRespectsHostAndSite(t *testing.T) {
	project := config.ProjectConfig{
		Apps: []config.AppProjectConfig{
			{ID: "a", Name: "storefront", Host: "api.nimbu.io", Site: "demo"},
			{ID: "b", Name: "other-site", Host: "api.nimbu.io", Site: "other"},
			{ID: "c", Name: "other-host", Host: "api.other.io", Site: "demo"},
			{ID: "d", Name: "global"},
		},
	}

	apps := VisibleApps("/tmp/project", project, "api.nimbu.io", "demo")
	if len(apps) != 2 {
		t.Fatalf("visible app count = %d", len(apps))
	}
	if apps[0].Name != "global" || apps[1].Name != "storefront" {
		t.Fatalf("unexpected apps: %#v", apps)
	}
}

// Regression: an app without a glob silently collected zero files.
func TestCollectFilesDefaultsToJSWithoutGlob(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"code/main.js", "code/lib/util.js", "code/readme.md"} {
		abs := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(abs, nil, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	files, err := CollectFiles(AppConfig{AppProjectConfig: config.AppProjectConfig{Dir: "code"}, ProjectRoot: root})
	if err != nil {
		t.Fatalf("collect files: %v", err)
	}
	if want := []string{"code/lib/util.js", "code/main.js"}; !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
}
