package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
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
