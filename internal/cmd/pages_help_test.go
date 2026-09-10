package cmd

import (
	"strings"
	"testing"
)

func TestPagesUpdateHelpListsSecurityMechanism(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")
	t.Setenv("NIMBU_COLOR", "never")

	code, stdout, stderr := captureExecute(t, []string{"pages", "update", "--help"})
	if code != 0 {
		t.Fatalf("pages update --help exit %d stderr=%q stdout=%q", code, stderr, stdout)
	}
	for _, needle := range []string{"security_mechanism", "none|humans|customers"} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("pages update --help missing %q\n%s", needle, stdout)
		}
	}
}

func TestPagesCreateHelpListsPublishedAndSecurityMechanism(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")
	t.Setenv("NIMBU_COLOR", "never")

	code, stdout, stderr := captureExecute(t, []string{"pages", "create", "--help"})
	if code != 0 {
		t.Fatalf("pages create --help exit %d stderr=%q stdout=%q", code, stderr, stdout)
	}
	for _, needle := range []string{"security_mechanism", "none|humans|customers", "published:=true"} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("pages create --help missing %q\n%s", needle, stdout)
		}
	}
}
