//go:build !windows

package cmd

import (
	"os"
	"os/signal"
	"syscall"
)

// serverShutdownSignals are the signals nimbu server owns while it runs.
// SIGPIPE is caught so a write to a closed stdout (`nimbu server | head`)
// fails with EPIPE instead of killing nimbu with the child orphaned and the
// terminal in no-echo mode; the stdout watch then stops the server. SIGQUIT
// (Ctrl-\) force-stops instead of dumping goroutines.
func serverShutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGPIPE, syscall.SIGQUIT}
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

// ignoreServerBackgroundReads makes a read from the terminal while nimbu
// runs in the background (Ctrl-Z, then `bg`) fail with EIO instead of
// stopping the whole process with SIGTTIN, which would freeze the proxy and
// hold a SIGTERM until `fg`. The shortcut listener pauses on that EIO. Call
// it after the child started: an ignored signal is inherited across exec.
// It stays ignored until nimbu exits; os/signal cannot reset an ignored
// signal it does not otherwise handle.
func ignoreServerBackgroundReads() {
	signal.Ignore(syscall.SIGTTIN)
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
