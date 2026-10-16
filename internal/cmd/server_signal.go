package cmd

import (
	"errors"
	"os"
)

// serverExitInterrupted is the conventional exit status for a run ended by
// Ctrl-C (128 + SIGINT); nimbu server uses it for a forced stop.
const serverExitInterrupted = 130

// errServerForceStopped ends Run after a second stop request. The notice was
// already printed, so the error itself stays silent.
var errServerForceStopped = &ExitError{
	Code: serverExitInterrupted,
	Err:  &displayedError{err: errors.New("server force-stopped")},
}

// serverStop runs the graceful stop in the background so the event loop can
// still react to a second Ctrl-C (or 'q') while the child and the proxy wind
// down; each may take up to its 5s grace period.
type serverStop struct {
	graceful func()
	done     chan struct{}
}

// Stopping reports whether a stop was requested.
func (s *serverStop) Stopping() bool {
	return s.done != nil
}

// Done is closed when the graceful stop has finished. It is nil, and so
// blocks forever in a select, before Begin.
func (s *serverStop) Done() <-chan struct{} {
	return s.done
}

// Begin starts the graceful stop. It returns false when a stop is already
// running, which means the caller should force the stop instead.
func (s *serverStop) Begin(term *serverShortcutListener) bool {
	if s.done != nil {
		return false
	}
	done := make(chan struct{})
	s.done = done
	go func() {
		defer term.RestoreOnPanic()
		defer close(done)
		if s.graceful != nil {
			s.graceful()
		}
	}()
	return true
}

// serverSignalName names a stop signal for the stopping notice; a stop asked
// for with the 'q' shortcut has none.
func serverSignalName(sig os.Signal) string {
	if sig == nil {
		return ""
	}
	return sig.String()
}

// serverForceHint says how to force the stop. Ctrl-C is only named when it
// can be pressed: stdin is a terminal, and the stop came from Ctrl-C itself
// or from the 'q' shortcut. A SIGTERM/SIGHUP from a supervisor is forced by
// sending it again.
func serverForceHint(sig os.Signal, stdinTTY bool) string {
	if stdinTTY && (sig == nil || sig == os.Interrupt) {
		return "Ctrl-C again to force"
	}
	return "send the signal again to force"
}
