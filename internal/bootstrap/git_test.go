//go:build !windows

package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bootstrap commit is nimbu's own, so the user's global hooks and commit
// signing must not run: a blocking hook or a pinentry prompt would hang init
// while the wizard holds every Ctrl-C.
func TestInitGitRepoIgnoresUserHooksAndSigning(t *testing.T) {
	home := t.TempDir()
	hooks := filepath.Join(home, "hooks")
	marker := filepath.Join(home, "hook-ran")
	for _, hook := range []string{"pre-commit", "commit-msg", "post-commit"} {
		writeTestFile(t, filepath.Join(hooks, hook), "#!/bin/sh\ntouch '"+marker+"'\nexit 1\n")
		if err := os.Chmod(filepath.Join(hooks, hook), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	globalConfig := filepath.Join(home, "gitconfig")
	writeTestFile(t, globalConfig, strings.Join([]string{
		"[core]",
		"\thooksPath = " + hooks,
		"[commit]",
		"\tgpgsign = true",
		"[gpg]",
		"\tprogram = false",
		"",
	}, "\n"))
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	repo := t.TempDir()
	writeTestFile(t, filepath.Join(repo, "nimbu.yml"), "site: demo\n")
	if err := initGitRepo(context.Background(), repo); err != nil {
		t.Fatalf("initGitRepo: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a user hook ran during the bootstrap commit")
	}
	if got := strings.TrimSpace(gitOutput(t, repo, "rev-list", "--all", "--count")); got != "1" {
		t.Fatalf("commits = %s, want 1", got)
	}
}
