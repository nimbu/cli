//go:build !windows

package cmd

import (
	"os"
	"os/signal"
	"syscall"
)

func serverShutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}

// watchServerSuspend keeps the shortcut input mode correct across job
// control: on Ctrl-Z (SIGTSTP) it hands the terminal back in its original
// mode and then stops nimbu; on fg (SIGCONT) it re-applies shortcut mode.
// The child runs in its own process group and keeps running meanwhile.
func watchServerSuspend(listener *serverShortcutListener) (stop func()) {
	if listener == nil {
		return func() {}
	}
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGTSTP, syscall.SIGCONT)
	done := make(chan struct{})
	go func() {
		defer listener.RestoreOnPanic()
		for {
			select {
			case <-done:
				return
			case sig := <-sigCh:
				handleServerSuspendSignal(sig, listener, func() {
					_ = syscall.Kill(syscall.Getpid(), syscall.SIGSTOP)
				})
			}
		}
	}()
	return func() {
		signal.Stop(sigCh)
		close(done)
	}
}

type serverSuspender interface {
	Suspend() error
	Resume() error
}

func handleServerSuspendSignal(sig os.Signal, term serverSuspender, stopSelf func()) {
	switch sig {
	case syscall.SIGTSTP:
		_ = term.Suspend()
		// Catching SIGTSTP replaced the default stop; do it ourselves.
		stopSelf()
	case syscall.SIGCONT:
		_ = term.Resume()
	}
}
