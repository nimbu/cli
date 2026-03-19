package themes

import (
	"os/exec"
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
