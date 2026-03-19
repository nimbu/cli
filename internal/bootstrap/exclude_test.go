package bootstrap

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCompileExcludePatternsDirectorySlash(t *testing.T) {
	matchers, err := compileExcludePatterns([]string{"bootstrap/"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{"bootstrap/manifest.yml", true},
		{"bootstrap/nested/file.txt", true},
		{"bootstrapper/x", false},
		{"src/bootstrap/other", false},
		{"bootstrap", false}, // file named "bootstrap" — not a directory match
	}
	for _, tt := range tests {
		if got := excludeMatches(matchers, tt.path); got != tt.want {
			t.Errorf("excludeMatches(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestCompileExcludePatternsWildcardNoSlash(t *testing.T) {
	matchers, err := compileExcludePatterns([]string{"*.md"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{"README.md", true},
		{"docs/GUIDE.md", true},
		{"a/b/c/DEEP.md", true},
		{"README.mdx", false},
		{"file.txt", false},
	}
	for _, tt := range tests {
		if got := excludeMatches(matchers, tt.path); got != tt.want {
			t.Errorf("excludeMatches(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestCompileExcludePatternsAnchoredWithSlash(t *testing.T) {
	matchers, err := compileExcludePatterns([]string{"content/notifications/Gemfile*"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{"content/notifications/Gemfile", true},
		{"content/notifications/Gemfile.lock", true},
		{"content/notifications/Gemfile.bak", true},
		{"other/content/notifications/Gemfile", false},
		{"content/notifications/README.md", false},
	}
	for _, tt := range tests {
		if got := excludeMatches(matchers, tt.path); got != tt.want {
			t.Errorf("excludeMatches(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestCompileExcludePatternsExactFilenameNoSlash(t *testing.T) {
	matchers, err := compileExcludePatterns([]string{"README.md"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{"README.md", true},
		{"docs/README.md", true},
		{"a/b/README.md", true},
		{"README.mdx", false},
		{"XREADME.md", false},
	}
	for _, tt := range tests {
		if got := excludeMatches(matchers, tt.path); got != tt.want {
			t.Errorf("excludeMatches(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestCompileExcludePatternsAnchoredExactPath(t *testing.T) {
	matchers, err := compileExcludePatterns([]string{"content/notifications/README.md"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{"content/notifications/README.md", true},
		{"README.md", false},
		{"other/content/notifications/README.md", false},
	}
	for _, tt := range tests {
		if got := excludeMatches(matchers, tt.path); got != tt.want {
			t.Errorf("excludeMatches(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestPathIsClaimed(t *testing.T) {
	paths := []string{
		"templates/customers/login.liquid",
		"snippets/repeatables/header.liquid",
		"src/recipes/tom-select",
	}

	tests := []struct {
		file string
		want bool
	}{
		{"templates/customers/login.liquid", true},
		{"snippets/repeatables/header.liquid", true},
		{"src/recipes/tom-select/alpine.ts", true},
		{"src/recipes/tom-select/index.css", true},
		{"src/recipes/tom-selector/other.ts", false},
		{"templates/page.liquid", false},
		{"nimbu.yml", false},
	}
	for _, tt := range tests {
		if got := pathIsClaimed(tt.file, paths); got != tt.want {
			t.Errorf("pathIsClaimed(%q) = %v, want %v", tt.file, got, tt.want)
		}
	}
}

func TestGitTrackedFiles(t *testing.T) {
	dir := t.TempDir()

	// Init git repo with a few files
	writeTestFile(t, filepath.Join(dir, "nimbu.yml"), "site: test\n")
	writeTestFile(t, filepath.Join(dir, "src", "index.ts"), "export {}\n")
	writeTestFile(t, filepath.Join(dir, "bootstrap", "manifest.yml"), "name: test\n")

	for _, args := range [][]string{
		{"init"},
		{"add", "."},
		{"-c", "user.name=test", "-c", "user.email=test@test", "commit", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	files, err := gitTrackedFiles(dir)
	if err != nil {
		t.Fatalf("gitTrackedFiles: %v", err)
	}

	want := map[string]bool{
		"nimbu.yml":              false,
		"src/index.ts":           false,
		"bootstrap/manifest.yml": false,
	}
	for _, f := range files {
		if _, ok := want[f]; ok {
			want[f] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("expected %q in git tracked files", path)
		}
	}
}

func TestGitTrackedFilesNotARepo(t *testing.T) {
	dir := t.TempDir()
	_, err := gitTrackedFiles(dir)
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func TestCompileExcludePatternsSkipsEmpty(t *testing.T) {
	matchers, err := compileExcludePatterns([]string{"", "  ", "*.md"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(matchers) != 1 {
		t.Fatalf("expected 1 matcher, got %d", len(matchers))
	}
	if !excludeMatches(matchers, "README.md") {
		t.Error("expected *.md to match README.md")
	}
}

func TestCompileExcludePatternsMultiplePatterns(t *testing.T) {
	matchers, err := compileExcludePatterns([]string{
		"bootstrap/",
		".github/",
		"*.md",
		"content/notifications/Gemfile*",
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	excluded := []string{
		"bootstrap/manifest.yml",
		".github/workflows/ci.yml",
		"README.md",
		"docs/GUIDE.md",
		"content/notifications/Gemfile",
		"content/notifications/Gemfile.lock",
	}
	for _, path := range excluded {
		if !excludeMatches(matchers, path) {
			t.Errorf("expected %q to be excluded", path)
		}
	}

	included := []string{
		"nimbu.yml",
		"src/index.ts",
		"templates/page.liquid",
		"content/notifications/email.liquid",
	}
	for _, path := range included {
		if excludeMatches(matchers, path) {
			t.Errorf("expected %q to NOT be excluded", path)
		}
	}
}
