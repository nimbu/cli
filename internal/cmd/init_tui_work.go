package cmd

import (
	"context"
	"sync"
	"time"
)

// initWork tracks the wizard's background jobs (clone, bootstrap) and the
// cleanups they register, so the command can stop and wait for them however the
// UI exits — success, esc, ctrl+c or a signal — before removing temp dirs.
//
// Without it a job could outlive the UI: an orphaned git clone keeps writing
// into a temp dir nobody removes, or a cleanup deletes the source tree while a
// bootstrap is still copying from it.
type initWork struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	closed   bool
	gaveUp   bool // shutdownWithin timed out; jobs may still be running
	wg       sync.WaitGroup
	cleanups []func()
}

func newInitWork(parent context.Context) *initWork {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &initWork{ctx: ctx, cancel: cancel}
}

// begin registers a job. It returns false once shutdown has started; the
// caller must then skip the job. Every successful begin needs a matching end.
func (w *initWork) begin() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	w.wg.Add(1)
	return true
}

func (w *initWork) end() { w.wg.Done() }

// onShutdown registers a cleanup that runs after every job has finished.
func (w *initWork) onShutdown(fn func()) {
	if fn == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cleanups = append(w.cleanups, fn)
}

// shutdown cancels the jobs' context, waits for running jobs to return, then
// runs the registered cleanups in reverse order. It is safe to call twice.
func (w *initWork) shutdown() { w.shutdownWithin(0) }

// shutdownWithin is shutdown with the wait bounded by timeout (0 waits for
// good). When a job is still running at the deadline it reports false and
// skips the cleanups, which could remove files the job still uses; later
// calls then return at once. nimbu is about to exit at that point.
func (w *initWork) shutdownWithin(timeout time.Duration) bool {
	w.mu.Lock()
	w.closed = true
	gaveUp := w.gaveUp
	w.mu.Unlock()
	if gaveUp {
		return false
	}

	w.cancel()
	if !w.wait(timeout) {
		w.mu.Lock()
		w.gaveUp = true
		w.mu.Unlock()
		return false
	}

	w.mu.Lock()
	cleanups := w.cleanups
	w.cleanups = nil
	w.mu.Unlock()
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
	return true
}

func (w *initWork) wait(timeout time.Duration) bool {
	if timeout <= 0 {
		w.wg.Wait()
		return true
	}
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}
