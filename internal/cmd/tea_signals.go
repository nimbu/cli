package cmd

import (
	"os"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// forwardProgramSignals claims SIGINT/SIGTERM from the root handler and
// delivers every one to program as Bubble Tea's own handler would
// (InterruptMsg/QuitMsg), for as long as the program runs. Run the program
// with tea.WithoutSignalHandler: Bubble Tea's handler stops after the first
// signal and, next to the root handler, would see each signal a second time.
//
// While claimed, the root neither cancels the context the program runs under
// nor force-exits, so the program decides how it ends (the init wizard holds
// signals while it writes files). If nimbu force-exits anyway, the program is
// killed first so the terminal leaves raw mode and the alternate screen.
// The returned stop function releases the signals; it is safe to call twice.
func forwardProgramSignals(program *tea.Program) func() {
	sigs, release := claimInterrupts()
	unregister := registerTerminalRestore(program.Kill)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case sig := <-sigs:
				// Send is a no-op once the program has exited.
				program.Send(programSignalMsg(sig))
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			release()
			unregister()
			close(done)
		})
	}
}

// programSignalMsg maps a signal to the message Bubble Tea's own handler sends.
func programSignalMsg(sig os.Signal) tea.Msg {
	if sig == os.Interrupt {
		return tea.InterruptMsg{}
	}
	return tea.QuitMsg{}
}
