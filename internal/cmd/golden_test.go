package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertGolden(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	want := string(wantBytes)
	norm := func(s string) string {
		s = strings.ReplaceAll(s, "\r\n", "\n")
		return strings.TrimRight(s, "\n")
	}
	if norm(got) != norm(want) {
		t.Fatalf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}
