//go:build !windows

package output

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestProgressInterruptRealSignal runs a child process that draws a live
// progress line and then receives a real SIGINT: the line must be erased and
// the process must still die from the signal (exit status 130 in a shell).
func TestProgressInterruptRealSignal(t *testing.T) {
	if os.Getenv("NIMBU_PROGRESS_SIGNAL_CHILD") == "1" {
		p := newProgress(os.Stderr, Theme{}, GlyphSet(true), true)
		p.mu.Lock()
		p.spinnerDelay = 0
		p.mu.Unlock()
		p.Phase("long work")
		for !p.lineIsOpen() {
			time.Sleep(5 * time.Millisecond)
		}
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		time.Sleep(5 * time.Second)
		os.Exit(0) // not reached when the signal kills us
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestProgressInterruptRealSignal$")
	cmd.Env = append(os.Environ(), "NIMBU_PROGRESS_SIGNAL_CHILD=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected child to die from SIGINT, got err=%v stderr=%q", err, stderr.String())
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGINT {
		t.Fatalf("expected death by SIGINT, got %v", exitErr)
	}
	if !strings.HasSuffix(stderr.String(), clearLine) {
		t.Fatalf("expected the live line to be erased before exit, got %q", stderr.String())
	}
}
