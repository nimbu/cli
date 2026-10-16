package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
)

// errInterrupted is the cancel cause of the root context after SIGINT/SIGTERM.
var errInterrupted = errors.New("interrupted")

// Seams for tests.
var (
	forceExit     = os.Exit
	notifySignals = func(ch chan<- os.Signal) { signal.Notify(ch, os.Interrupt, syscall.SIGTERM) }
	stopSignals   = func(ch chan<- os.Signal) { signal.Stop(ch) }
)

// withInterrupt derives a context that is canceled with errInterrupted on the
// first SIGINT or SIGTERM, so long operations (theme pushes, site copies,
// uploads) unwind cleanly. A second signal exits at once with 130 for a
// command that does not watch its context. Commands that install their own
// signal.NotifyContext still receive every signal: Go delivers each signal to
// all registered channels.
func withInterrupt(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	sigCh := make(chan os.Signal, 2)
	notifySignals(sigCh)
	done := make(chan struct{})
	go watchInterrupts(sigCh, done, cancel)
	return ctx, func() {
		stopSignals(sigCh)
		close(done)
		cancel(nil)
	}
}

func watchInterrupts(sigCh <-chan os.Signal, done <-chan struct{}, cancel context.CancelCauseFunc) {
	select {
	case <-sigCh:
		cancel(errInterrupted)
	case <-done:
		return
	}
	select {
	case <-sigCh:
		forceExit(ExitInterrupted)
	case <-done:
	}
}

// interrupted reports whether ctx was canceled by a signal.
func interrupted(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), errInterrupted)
}

// interruptedError marks a command failure that followed an interrupt: the
// failure is the interrupt's doing, so it exits 130 whatever it wraps.
type interruptedError struct{ err error }

func (e *interruptedError) Error() string {
	if e.err == nil {
		return errInterrupted.Error()
	}
	return e.err.Error()
}

func (e *interruptedError) Unwrap() []error { return []error{errInterrupted, e.err} }
