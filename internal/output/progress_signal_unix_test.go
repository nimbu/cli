//go:build !windows

package output

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Regression: with a live progress line on screen, one Ctrl-C used to reach a
// root-style handler twice (the progress handler re-raised it), so the first
// press already force-exited 130 and skipped all cleanup. The child below
// mimics the root contract — cancel on the first signal, exit 130 on the
// second — and sends exactly one SIGINT.
func TestProgressOneInterruptIsSeenOnceByRootHandler(t *testing.T) {
	if os.Getenv("NIMBU_PROGRESS_ROOT_SIGNAL_CHILD") == "1" {
		ctx, cancel := context.WithCancel(context.Background())
		rootCh := make(chan os.Signal, 4)
		signal.Notify(rootCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			seen := 0
			for range rootCh {
				seen++
				if seen == 1 {
					cancel()
					continue
				}
				_, _ = os.Stderr.WriteString("FORCED")
				os.Exit(130)
			}
		}()
		p := newProgress(ctx.Done(), os.Stderr, Theme{}, GlyphSet(true), true)
		p.mu.Lock()
		p.spinnerDelay = 0
		p.mu.Unlock()
		p.Phase("long work")
		for !p.lineIsOpen() {
			time.Sleep(5 * time.Millisecond)
		}
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		<-ctx.Done()
		<-p.doneCh
		time.Sleep(300 * time.Millisecond) // room for a stray second delivery
		p.Close()
		_, _ = os.Stderr.WriteString("CLEAN")
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestProgressOneInterruptIsSeenOnceByRootHandler$")
	cmd.Env = append(os.Environ(), "NIMBU_PROGRESS_ROOT_SIGNAL_CHILD=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run child: %v", err)
	}
	out := stderr.String()
	if code != 0 || strings.Contains(out, "FORCED") {
		t.Fatalf("one SIGINT force-exited: exit=%d stderr=%q", code, out)
	}
	if !strings.HasSuffix(out, clearLine+"CLEAN") {
		t.Fatalf("expected the live line erased before cleanup output, got %q", out)
	}
}
