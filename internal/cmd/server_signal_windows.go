//go:build windows

package cmd

import (
	"os"
	"syscall"
)

func serverShutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

// watchServerSuspend is a no-op: Windows has no job-control suspend.
func watchServerSuspend(*serverShortcutListener) (stop func()) {
	return func() {}
}
