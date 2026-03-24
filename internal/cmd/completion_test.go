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

	// Read in a goroutine to avoid deadlock on Windows where pipe buffers are small.
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&buf, r)
		done <- copyErr
	}()

	callErr := fn()
	_ = w.Close()
	os.Stdout = origStdout

	if readErr := <-done; readErr != nil {
		t.Fatalf("read pipe: %v", readErr)
	}
	if callErr != nil {
		t.Fatalf("call failed: %v", callErr)
	}

	return buf.String()
}

func TestCompletionsIncludeNewTopLevelCommands(t *testing.T) {
	required := []string{
		"accounts",
		"collections",
		"coupons",
		"mails",
		"notifications",
		"roles",
		"redirects",
		"functions",
		"jobs",
		"apps",
		"translations",
		"info",
		"pull",
		"diff",
		"copy",
		"push",
		"sync",
		"config",
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

func TestCompletionsIncludeCustomerAndProductFields(t *testing.T) {
	bash := captureStdout(t, func() error { return writeBashCompletion(nil) })
	zsh := captureStdout(t, func() error { return writeZshCompletion(nil) })
	fish := captureStdout(t, func() error { return writeFishCompletion(nil) })

	for _, needle := range []string{
		`list get create update delete count copy fields config`,
		`list get create update delete count fields config`,
		`fields:Show customer field schema`,
		`fields:Show product field schema`,
	} {
		if !strings.Contains(bash, needle) && !strings.Contains(zsh, needle) && !strings.Contains(fish, needle) {
			t.Fatalf("expected completion output to include %q", needle)
		}
	}
}

func TestCompletionsUseRenamedCommandAndAlias(t *testing.T) {
	bash := captureStdout(t, func() error { return writeBashCompletion(nil) })
	zsh := captureStdout(t, func() error { return writeZshCompletion(nil) })
	fish := captureStdout(t, func() error { return writeFishCompletion(nil) })

	for name, out := range map[string]string{
		"bash": bash,
		"zsh":  zsh,
		"fish": fish,
	} {
		if strings.Contains(out, "nimbu-cli") {
			t.Fatalf("%s completion should not mention old command name: %q", name, out)
		}
		if !strings.Contains(out, "nimbu") {
			t.Fatalf("%s completion should mention renamed command", name)
		}
		if !strings.Contains(out, "nb") {
			t.Fatalf("%s completion should keep nb alias", name)
		}
	}
}
