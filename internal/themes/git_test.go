package themes

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCollectGitChangesNoHeadFallback(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	cfg := Config{ProjectRoot: dir}

	changes, err := CollectGitChanges(cfg, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changes.FallbackAll {
		t.Fatal("expected FallbackAll when no HEAD and since is empty")
	}
}

func TestCollectGitChangesSinceErrorsNoHead(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	cfg := Config{ProjectRoot: dir}

	_, err := CollectGitChanges(cfg, "origin/main")
	if err == nil {
		t.Fatal("expected error when --since is set but repo has no commits")
	}
	if !strings.Contains(err.Error(), "no commits") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCollectGitChangesResolvesSymlinkedProjectRoot(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "theme")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	writeThemeTestFile(t, real, "snippets/svg/a.liquid", "a")
	initThemeGitRepo(t, real)
	runGit(t, real, "add", "snippets/svg/a.liquid")
	runGit(t, real, "-c", "user.name=test", "-c", "user.email=test@test", "commit", "-m", "snippet")
	writeThemeTestFile(t, real, "templates/page.liquid", "page")

	changes, err := CollectGitChanges(themeAllRootsTestConfig(link), "HEAD")
	if err != nil {
		t.Fatalf("CollectGitChanges: %v", err)
	}
	if !reflect.DeepEqual(changes.Changed, []string{"templates/page.liquid"}) {
		t.Fatalf("changed = %#v, want the untracked template", changes.Changed)
	}
}
