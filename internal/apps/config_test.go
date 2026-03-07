package apps

import (
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
