//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

func serverShutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}
