package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	callErr := fn()
	_ = w.Close()
	os.Stdout = origStdout

	if callErr != nil {
		t.Fatalf("call failed: %v", callErr)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	return buf.String()
}

func TestCompletionsIncludeNewTopLevelCommands(t *testing.T) {
	required := []string{
		"accounts",
		"collections",
		"coupons",
		"notifications",
		"roles",
		"redirects",
		"functions",
		"jobs",
		"apps",
		"translations",
		"push",
		"sync",
	}

	bash := captureStdout(t, func() error { return writeBashCompletion(nil) })
	zsh := captureStdout(t, func() error { return writeZshCompletion(nil) })
	fish := captureStdout(t, func() error { return writeFishCompletion(nil) })

	for _, cmd := range required {
		if !strings.Contains(bash, cmd) {
			t.Fatalf("bash completion missing %q", cmd)
		}
		if !strings.Contains(zsh, cmd) {
			t.Fatalf("zsh completion missing %q", cmd)
		}
		if !strings.Contains(fish, cmd) {
			t.Fatalf("fish completion missing %q", cmd)
		}
	}
}
