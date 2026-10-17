package cmd

import (
	"strings"
	"testing"
)

// H7: the shell is accepted as a positional argument and as --shell.
func TestCompletionAcceptsShellArgument(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	run := func(args ...string) string {
		t.Helper()
		code, stdout, stderr := captureExecute(t, args)
		if code != 0 {
			t.Fatalf("%v: exit %d stderr=%q", args, code, stderr)
		}
		return stdout
	}
	for _, args := range [][]string{{"completion", "zsh"}, {"completion", "--shell=zsh"}, {"completion", "zsh", "--shell", "zsh"}} {
		if out := run(args...); !strings.HasPrefix(out, "#compdef nimbu") {
			t.Fatalf("%v: not a zsh script:\n%.80s", args, out)
		}
	}
	if out := run("completion"); !strings.HasPrefix(out, "# Bash completion") {
		t.Fatalf("default shell is not bash:\n%.80s", out)
	}
	if out := run("completion", "fish"); !strings.HasPrefix(out, "# Fish completion") {
		t.Fatalf("fish:\n%.80s", out)
	}
	for _, args := range [][]string{{"completion", "zsh", "--shell=fish"}, {"completion", "tcsh"}} {
		code, _, stderr := captureExecute(t, args)
		if code != ExitUsage {
			t.Fatalf("%v: exit %d stderr=%q", args, code, stderr)
		}
	}
}
