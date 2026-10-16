package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/auth"
)

func stubForceExit(t *testing.T) <-chan int {
	t.Helper()
	codes := make(chan int, 1)
	original := forceExit
	forceExit = func(code int) { codes <- code }
	t.Cleanup(func() { forceExit = original })
	return codes
}

func TestWatchInterruptsCancelsThenForceExits(t *testing.T) {
	codes := stubForceExit(t)
	sigCh := make(chan os.Signal, 2)
	done := make(chan struct{})
	ctx, cancel := context.WithCancelCause(context.Background())
	finished := make(chan struct{})
	go func() {
		watchInterrupts(sigCh, done, cancel)
		close(finished)
	}()

	sigCh <- os.Interrupt
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("first signal did not cancel the context")
	}
	if !interrupted(ctx) {
		t.Fatalf("cause = %v, want errInterrupted", context.Cause(ctx))
	}
	select {
	case code := <-codes:
		t.Fatalf("first signal must not exit, got %d", code)
	default:
	}

	sigCh <- syscall.SIGTERM
	select {
	case code := <-codes:
		if code != ExitInterrupted {
			t.Fatalf("second signal exit = %d, want %d", code, ExitInterrupted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second signal did not force an exit")
	}
	<-finished
}

func TestWithInterruptStopsCleanlyWithoutSignal(t *testing.T) {
	codes := stubForceExit(t)
	var mu sync.Mutex
	var registered, stopped int
	origNotify, origStop := notifySignals, stopSignals
	notifySignals = func(chan<- os.Signal) { mu.Lock(); registered++; mu.Unlock() }
	stopSignals = func(chan<- os.Signal) { mu.Lock(); stopped++; mu.Unlock() }
	t.Cleanup(func() { notifySignals, stopSignals = origNotify, origStop })

	ctx, stop := withInterrupt(context.Background())
	stop()
	if interrupted(ctx) {
		t.Fatal("a normal finish must not look interrupted")
	}
	mu.Lock()
	defer mu.Unlock()
	if registered != 1 || stopped != 1 {
		t.Fatalf("notify=%d stop=%d, want 1 each", registered, stopped)
	}
	select {
	case code := <-codes:
		t.Fatalf("unexpected exit %d", code)
	default:
	}
}

func TestInterruptedErrorsExit130(t *testing.T) {
	for _, err := range []error{
		&interruptedError{err: errors.New("upload failed")},
		fmt.Errorf("push theme: %w", context.Canceled),
	} {
		desc := classifyError(err)
		if desc.Code != errorInterrupted || desc.ExitCode != ExitInterrupted || desc.Retryable {
			t.Fatalf("classifyError(%v) = %+v", err, desc)
		}
	}
	wrapped := &interruptedError{err: errors.New("upload failed")}
	if !errors.Is(wrapped, errInterrupted) || wrapped.Error() != "upload failed" {
		t.Fatalf("interruptedError must wrap both causes: %v", wrapped)
	}
}

func TestInterruptDuringPromptExitsImmediately(t *testing.T) {
	codes := stubForceExit(t)
	endPrompt := beginPrompt()
	defer endPrompt()

	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	defer close(done)
	ctx, cancel := context.WithCancelCause(context.Background())
	go watchInterrupts(sigCh, done, cancel)

	sigCh <- os.Interrupt
	select {
	case code := <-codes:
		if code != ExitInterrupted {
			t.Fatalf("exit = %d, want %d", code, ExitInterrupted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ctrl-C during a prompt must exit at once")
	}
	if !interrupted(ctx) {
		t.Fatal("context should still be canceled")
	}
}

// startTestWatcher runs watchInterrupts on a fake signal channel with the root
// coordinator marked active, as withInterrupt does in production.
func startTestWatcher(t *testing.T) (chan os.Signal, context.Context) {
	t.Helper()
	sigCh := make(chan os.Signal, 4)
	done := make(chan struct{})
	finished := make(chan struct{})
	ctx, cancel := context.WithCancelCause(context.Background())
	rootInterrupts.setActive(true)
	go func() {
		defer close(finished)
		watchInterrupts(sigCh, done, cancel)
	}()
	t.Cleanup(func() {
		close(done)
		<-finished
		rootInterrupts.setActive(false)
	})
	return sigCh, ctx
}

// A command that claimed the signals (nimbu server, a Bubble Tea program)
// owns every one of them: the root must neither cancel its context nor
// force-exit, however many arrive, and takes over again after release.
func TestClaimedSignalsBypassRootCancelAndForceExit(t *testing.T) {
	codes := stubForceExit(t)
	sigCh, ctx := startTestWatcher(t)

	claimed, release := claimInterrupts()
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM, os.Interrupt} {
		sigCh <- sig
		select {
		case got := <-claimed:
			if got != sig {
				t.Fatalf("claim got %v, want %v", got, sig)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%v was not forwarded to the claim", sig)
		}
	}
	if ctx.Err() != nil {
		t.Fatal("root canceled its context while the signals were claimed")
	}
	select {
	case code := <-codes:
		t.Fatalf("root force-exited %d while the signals were claimed", code)
	default:
	}

	release()
	release() // idempotent
	sigCh <- os.Interrupt
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("root did not take the first signal back after release")
	}
	select {
	case code := <-codes:
		t.Fatalf("first signal after release force-exited %d", code)
	default:
	}
	select {
	case <-claimed:
		t.Fatal("released claim still received a signal")
	default:
	}
}

// Regression: the second signal called os.Exit(130) straight away, leaving a
// terminal in raw/no-echo mode. Restores run newest first, a panicking one
// does not stop the rest, and a stuck one cannot block the exit.
func TestForceExitRunsTerminalRestoresBounded(t *testing.T) {
	codes := stubForceExit(t)
	origTimeout := terminalRestoreTimeout
	terminalRestoreTimeout = 200 * time.Millisecond
	t.Cleanup(func() { terminalRestoreTimeout = origTimeout })

	var mu sync.Mutex
	var order []string
	record := func(name string) func() {
		return func() { mu.Lock(); order = append(order, name); mu.Unlock() }
	}
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	defer registerTerminalRestore(func() { <-block })()
	defer registerTerminalRestore(record("first"))()
	unregistered := registerTerminalRestore(record("unregistered"))
	defer registerTerminalRestore(func() { panic("boom") })()
	defer registerTerminalRestore(record("last"))()
	unregistered()

	sigCh, _ := startTestWatcher(t)
	sigCh <- os.Interrupt
	sigCh <- os.Interrupt
	start := time.Now()
	select {
	case code := <-codes:
		if code != ExitInterrupted {
			t.Fatalf("exit = %d, want %d", code, ExitInterrupted)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second signal did not force an exit")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("a stuck restore delayed the exit by %s", elapsed)
	}
	mu.Lock()
	defer mu.Unlock()
	if got := strings.Join(order, ","); got != "last,first" {
		t.Fatalf("restores ran %q, want newest first and none unregistered", got)
	}
}

// Ctrl-C during a prompt exits at once; the terminal restores run first (a
// password prompt has echo turned off at that moment).
func TestInterruptDuringPromptRunsRestores(t *testing.T) {
	codes := stubForceExit(t)
	restored := make(chan struct{}, 1)
	defer registerTerminalRestore(func() { restored <- struct{}{} })()
	endPrompt := beginPrompt()
	defer endPrompt()

	sigCh, _ := startTestWatcher(t)
	sigCh <- os.Interrupt
	select {
	case <-codes:
	case <-time.After(2 * time.Second):
		t.Fatal("Ctrl-C during a prompt must exit at once")
	}
	select {
	case <-restored:
	default:
		t.Fatal("prompt exit skipped the terminal restores")
	}
}

// Regression: the keyring file-backend password prompt blocked on stdin
// without being marked, so the first Ctrl-C only canceled a context the read
// never sees and a second Ctrl-C was needed.
func TestKeyringPromptHookMarksPrompt(t *testing.T) {
	before := activePrompts.Load()
	end := auth.BeginPrompt()
	if got := activePrompts.Load(); got != before+1 {
		t.Fatalf("auth.BeginPrompt did not mark a prompt: %d -> %d", before, got)
	}
	end()
	end() // idempotent
	if got := activePrompts.Load(); got != before {
		t.Fatalf("prompt not unmarked: %d, want %d", got, before)
	}
}

// Regression: a prompt that starts after the first signal already canceled the
// context (a command asking during its cleanup) was force-exited by the next
// Ctrl-C without ending the prompt line, so the shell prompt landed on it.
func TestInterruptDuringLatePromptEndsPromptLine(t *testing.T) {
	codes := stubForceExit(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = origStderr
		_ = r.Close()
	})

	sigCh, ctx := startTestWatcher(t)
	sigCh <- os.Interrupt
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("first signal did not cancel the context")
	}

	endPrompt := beginPrompt()
	defer endPrompt()
	sigCh <- os.Interrupt
	select {
	case <-codes:
	case <-time.After(2 * time.Second):
		t.Fatal("Ctrl-C during the prompt did not exit")
	}
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if string(out) != "\n" {
		t.Fatalf("stderr = %q, want the prompt line ended", out)
	}
}
