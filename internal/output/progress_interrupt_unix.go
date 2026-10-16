//go:build !windows

package output

import (
	"os"
	"syscall"
)

var interruptSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// raiseSignal re-delivers sig to this process. The progress handler has
// already been stopped, so the runtime falls back to the default action (or
// to any other handler the command registered).
func raiseSignal(sig os.Signal) {
	if s, ok := sig.(syscall.Signal); ok {
		_ = syscall.Kill(syscall.Getpid(), s)
	}
}
