package cmd

import (
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/themes"
)

func TestThemeResourcePathPreservesFolderForNestedResources(t *testing.T) {
	got := themeResourcePath(themes.KindTemplate, api.ThemeResource{
		Folder: "customers",
		Name:   "login.liquid",
	})
	if got != "templates/customers/login.liquid" {
		t.Fatalf("themeResourcePath = %q, want nested template path", got)
	}
}
