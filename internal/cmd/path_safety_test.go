package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestNoRawDynamicPathConcatenation(t *testing.T) {
	pattern := regexp.MustCompile(`"/[a-z][^"]*"\s*\+\s*c\.`)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob files: %v", err)
	}

	for _, file := range files {
		if file == "path_safety_test.go" {
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}

		if pattern.Match(content) {
			t.Fatalf("raw dynamic path concatenation found in %s", file)
		}
	}
}

func TestSitesGetUsesPathEscape(t *testing.T) {
	content, err := os.ReadFile("sites_get.go")
	if err != nil {
		t.Fatalf("read sites_get.go: %v", err)
	}

	if !strings.Contains(string(content), "url.PathEscape(site)") {
		t.Fatal("sites_get.go must path-escape site identifier")
	}
}
