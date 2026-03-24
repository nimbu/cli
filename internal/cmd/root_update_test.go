package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/updatecheck"
)

func TestExecuteSkipsUpdateNotifierForJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	restore := stubUpdateNotifier(t, true, func(context.Context, io.Writer, string, updatecheck.Style) {
		t.Fatal("update notifier should not run")
	})
	defer restore()

	code, _, stderr := captureExecute(t, []string{"--json", "config", "path"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d with stderr %q", code, stderr)
	}
}

func TestExecuteSkipsUpdateNotifierForPlain(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	restore := stubUpdateNotifier(t, true, func(context.Context, io.Writer, string, updatecheck.Style) {
		t.Fatal("update notifier should not run")
	})
	defer restore()

	code, _, stderr := captureExecute(t, []string{"--plain", "config", "path"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d with stderr %q", code, stderr)
	}
}

func TestExecuteSkipsUpdateNotifierForNoInput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	restore := stubUpdateNotifier(t, true, func(context.Context, io.Writer, string, updatecheck.Style) {
		t.Fatal("update notifier should not run")
	})
	defer restore()

	code, _, stderr := captureExecute(t, []string{"--no-input", "config", "path"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d with stderr %q", code, stderr)
	}
}

func TestExecuteSkipsUpdateNotifierForCompletion(t *testing.T) {
	restore := stubUpdateNotifier(t, true, func(context.Context, io.Writer, string, updatecheck.Style) {
		t.Fatal("update notifier should not run")
	})
	defer restore()

	code, _, stderr := captureExecute(t, []string{"completion", "bash"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d with stderr %q", code, stderr)
	}
}

func TestExecuteSkipsUpdateNotifierForCompletionWithGlobalFlagPrefix(t *testing.T) {
	restore := stubUpdateNotifier(t, true, func(context.Context, io.Writer, string, updatecheck.Style) {
		t.Fatal("update notifier should not run")
	})
	defer restore()

	code, _, stderr := captureExecute(t, []string{"--color=never", "completion", "bash"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d with stderr %q", code, stderr)
	}
}

func TestExecuteRunsUpdateNotifierAfterSuccessfulCommandAndWritesOnlyToStderr(t *testing.T) {
	tempHome := t.TempDir()
	configHome := filepath.Join(tempHome, ".config")
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	if runtime.GOOS == "windows" {
		// Windows uses %APPDATA% instead of XDG_CONFIG_HOME.
		t.Setenv("APPDATA", configHome)
	}

	restore := stubUpdateNotifier(t, true, func(ctx context.Context, errWriter io.Writer, currentVersion string, style updatecheck.Style) {
		if currentVersion == "" {
			t.Fatal("expected current version")
		}
		if style.Bold == nil || style.Dim == nil {
			t.Fatal("expected styled notice functions")
		}
		_, _ = io.WriteString(errWriter, "update msg\n")
	})
	defer restore()
	restoreVersion := stubVersion(t, "v0.1.1")
	defer restoreVersion()

	code, stdout, stderr := captureExecute(t, []string{"config", "path"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d with stderr %q", code, stderr)
	}

	expectedPath := filepath.Join(configHome, "nimbu", "config.json")
	if !strings.Contains(stdout, expectedPath) {
		t.Fatalf("expected stdout to contain config path %q, got %q", expectedPath, stdout)
	}
	if strings.Contains(stdout, "update msg") {
		t.Fatalf("expected update notice to stay off stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "update msg") {
		t.Fatalf("expected stderr to contain update notice, got %q", stderr)
	}
}

func TestExecuteDoesNotRunUpdateNotifierWhenCommandFails(t *testing.T) {
	restore := stubUpdateNotifier(t, true, func(context.Context, io.Writer, string, updatecheck.Style) {
		t.Fatal("update notifier should not run")
	})
	defer restore()

	code, _, stderr := captureExecute(t, []string{"bogus"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code, got 0 with stderr %q", stderr)
	}
}

func stubVersion(t *testing.T, value string) func() {
	t.Helper()

	orig := version
	version = value
	return func() {
		version = orig
	}
}

func stubUpdateNotifier(t *testing.T, tty bool, fn func(context.Context, io.Writer, string, updatecheck.Style)) func() {
	t.Helper()

	origNotify := notifyUpdate
	origTTY := stderrIsTTY
	notifyUpdate = fn
	stderrIsTTY = func(*output.Writer) bool { return tty }

	return func() {
		notifyUpdate = origNotify
		stderrIsTTY = origTTY
	}
}

func captureExecute(t *testing.T, args []string) (int, string, string) {
	t.Helper()

	origStdout := os.Stdout
	origStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	// Read both pipes in goroutines to avoid deadlock on Windows where
	// pipe buffers are small and sequential reads can block.
	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutDone := make(chan error, 1)
	stderrDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&stdoutBuf, stdoutR)
		stdoutDone <- copyErr
	}()
	go func() {
		_, copyErr := io.Copy(&stderrBuf, stderrR)
		stderrDone <- copyErr
	}()

	code := Execute(args)

	_ = stdoutW.Close()
	_ = stderrW.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr

	if err := <-stdoutDone; err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	if err := <-stderrDone; err != nil {
		t.Fatalf("read stderr pipe: %v", err)
	}
	return code, stdoutBuf.String(), stderrBuf.String()
}
