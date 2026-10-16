package cmd

import (
	"context"
	"sync"
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
func (w *initWork) shutdown() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()

	w.cancel()
	w.wg.Wait()

	w.mu.Lock()
	cleanups := w.cleanups
	w.cleanups = nil
	w.mu.Unlock()
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
}
