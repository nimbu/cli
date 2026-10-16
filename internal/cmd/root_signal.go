package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/nimbu/cli/internal/auth"
)

// errInterrupted is the cancel cause of the root context after SIGINT/SIGTERM.
var errInterrupted = errors.New("interrupted")

// rootSignals are the signals the root handler owns.
var rootSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// Seams for tests.
var (
	forceExit     = os.Exit
	notifySignals = func(ch chan<- os.Signal) { signal.Notify(ch, rootSignals...) }
	stopSignals   = func(ch chan<- os.Signal) { signal.Stop(ch) }
	// terminalRestoreTimeout bounds the restores a forced exit runs, so a
	// stuck restore never keeps a second Ctrl-C from ending the process.
	terminalRestoreTimeout = time.Second
)

func init() {
	// The keyring's file-backend password prompt blocks on stdin like
	// prompt() does; internal/auth cannot import this package, so it calls
	// back through a hook.
	auth.BeginPrompt = beginPrompt
}

// Signal ownership: the root handler is the only subscriber to SIGINT and
// SIGTERM. Everything else either watches the root context, which is
// canceled on the first signal (progress lines, uploads, apps logs, channel
// watches), or claims the signals with claimInterrupts when it must decide
// itself what a signal means (nimbu server, the Bubble Tea programs). One
// keypress is therefore seen exactly once.
//
// withInterrupt derives a context that is canceled with errInterrupted on the
// first SIGINT or SIGTERM, so long operations unwind cleanly. A second signal
// runs the registered terminal restores and exits at once with 130, for a
// command that does not watch its context.
func withInterrupt(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	sigCh := make(chan os.Signal, 2)
	notifySignals(sigCh)
	rootInterrupts.setActive(true)
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		watchInterrupts(sigCh, done, cancel)
	}()
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			stopSignals(sigCh)
			close(done)
			<-finished
			rootInterrupts.setActive(false)
			cancel(nil)
		})
	}
}

// activePrompts counts prompts blocked on stdin. A read cannot be canceled
// through the context, so Ctrl-C during a prompt exits at once, as it did
// before the root installed a handler.
var activePrompts atomic.Int32

// beginPrompt marks a blocking stdin read; call the returned func when done.
// While it is marked, the first Ctrl-C exits 130 at once, after restoring the
// terminal mode the prompt started from (a password read turns echo off).
func beginPrompt() func() {
	activePrompts.Add(1)
	unregister := registerStdinRestore()
	var once sync.Once
	return func() {
		once.Do(func() {
			unregister()
			activePrompts.Add(-1)
		})
	}
}

func watchInterrupts(sigCh <-chan os.Signal, done <-chan struct{}, cancel context.CancelCauseFunc) {
	seen := 0
	for {
		select {
		case <-done:
			return
		case sig := <-sigCh:
			if rootInterrupts.forward(sig) {
				// A command claimed the signals: it decides what this one
				// means (graceful stop, force, cancel a dialog).
				continue
			}
			seen++
			prompting := activePrompts.Load() > 0
			if seen == 1 {
				cancel(errInterrupted)
				if !prompting {
					continue
				}
			}
			if prompting {
				// End the prompt line, also when a prompt started after
				// the first signal canceled the context.
				_, _ = os.Stderr.WriteString("\n")
			}
			exitInterrupted()
			return
		}
	}
}

// exitInterrupted restores the terminal (bounded, best-effort) and exits 130.
// Deferred cleanup does not run on this path, so anything that changes the
// terminal mode registers its restore with registerTerminalRestore.
func exitInterrupted() {
	runTerminalRestores(terminalRestoreTimeout)
	forceExit(ExitInterrupted)
}

// interruptCoordinator routes root signals to a command that claimed them and
// keeps the terminal restores a forced exit must run.
type interruptCoordinator struct {
	mu       sync.Mutex
	active   int // running root handlers (withInterrupt)
	claims   []chan os.Signal
	restores []*terminalRestore
}

type terminalRestore struct{ fn func() }

var rootInterrupts = &interruptCoordinator{}

func (c *interruptCoordinator) setActive(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if on {
		c.active++
	} else if c.active > 0 {
		c.active--
	}
}

// forward hands sig to the newest claim. It reports false when nothing
// claimed the signals and the root must handle it.
func (c *interruptCoordinator) forward(sig os.Signal) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.claims) == 0 {
		return false
	}
	select {
	case c.claims[len(c.claims)-1] <- sig:
	default:
		// The claimant still has signals queued; it already knows.
	}
	return true
}

// claimInterrupts hands SIGINT/SIGTERM, plus any other signal listed in sigs
// (SIGHUP for nimbu server), to the caller until release is called. While
// claimed the root neither cancels its context nor force-exits: the claimant
// owns the meaning of every signal, including "a second Ctrl-C forces".
// Without a root handler (a command run directly in tests) the signals are
// subscribed directly, so the claimant behaves the same.
func claimInterrupts(sigs ...os.Signal) (<-chan os.Signal, func()) {
	if len(sigs) == 0 {
		sigs = rootSignals
	}
	ch := make(chan os.Signal, 4)
	c := rootInterrupts

	c.mu.Lock()
	viaRoot := c.active > 0
	if viaRoot {
		c.claims = append(c.claims, ch)
	}
	c.mu.Unlock()

	var direct []os.Signal
	for _, sig := range sigs {
		if !viaRoot || !isRootSignal(sig) {
			direct = append(direct, sig)
		}
	}
	if len(direct) > 0 {
		signal.Notify(ch, direct...)
	}

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			if len(direct) > 0 {
				signal.Stop(ch)
			}
			if !viaRoot {
				return
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			for i := len(c.claims) - 1; i >= 0; i-- {
				if c.claims[i] == ch {
					c.claims = append(c.claims[:i], c.claims[i+1:]...)
					break
				}
			}
		})
	}
}

func isRootSignal(sig os.Signal) bool {
	for _, root := range rootSignals {
		if sig == root {
			return true
		}
	}
	return false
}

// registerTerminalRestore registers fn to run when nimbu force-exits (second
// signal, or Ctrl-C during a prompt) while the terminal is in a changed mode:
// raw or no-echo input, a running Bubble Tea program. Call the returned func
// once the mode was restored the normal way.
func registerTerminalRestore(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	entry := &terminalRestore{fn: fn}
	c := rootInterrupts
	c.mu.Lock()
	c.restores = append(c.restores, entry)
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			for i := len(c.restores) - 1; i >= 0; i-- {
				if c.restores[i] == entry {
					c.restores = append(c.restores[:i], c.restores[i+1:]...)
					break
				}
			}
		})
	}
}

// registerStdinRestore snapshots the stdin terminal mode and registers its
// restore for a forced exit, for code about to change that mode (a password
// read turns echo off; a terminal query switches to raw mode). It is a no-op
// when stdin is not a terminal. Call the returned func once the mode is back.
func registerStdinRestore() func() {
	fd := int(os.Stdin.Fd())
	state, err := term.GetState(fd)
	if err != nil {
		return func() {}
	}
	return registerTerminalRestore(func() { _ = term.Restore(fd, state) })
}

// runTerminalRestores runs the registered restores, newest first, waiting at
// most timeout. A panicking restore does not stop the others.
func runTerminalRestores(timeout time.Duration) {
	c := rootInterrupts
	c.mu.Lock()
	restores := c.restores
	c.restores = nil
	c.mu.Unlock()
	if len(restores) == 0 {
		return
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for i := len(restores) - 1; i >= 0; i-- {
			func() {
				defer func() { _ = recover() }()
				restores[i].fn()
			}()
		}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-finished:
	case <-timer.C:
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
