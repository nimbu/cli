//go:build !windows

package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type initSignalCountModel struct{}

func (initSignalCountModel) Init() tea.Cmd                         { return nil }
func (m initSignalCountModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (initSignalCountModel) View() tea.View                        { return tea.NewView("") }

// Bubble Tea's own handler stops listening after the first signal, so a
// second SIGTERM during the file write used to kill nimbu mid-copy with the
// terminal still raw. forwardInitSignals keeps delivering every signal.
func TestForwardInitSignalsDeliversRepeatedSignals(t *testing.T) {
	quits := make(chan struct{}, 4)
	filter := func(_ tea.Model, msg tea.Msg) tea.Msg {
		if _, ok := msg.(tea.QuitMsg); ok {
			quits <- struct{}{}
			if len(quits) < 2 {
				return nil // held, like the wizard does while writing files
			}
		}
		return msg
	}
	program := tea.NewProgram(initSignalCountModel{},
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
		tea.WithoutRenderer(),
		tea.WithoutSignalHandler(),
		tea.WithFilter(filter),
	)
	stop := forwardInitSignals(program)
	defer stop()

	done := make(chan error, 1)
	go func() {
		_, err := program.Run()
		done <- err
	}()

	for i := 0; i < 2; i++ {
		time.Sleep(50 * time.Millisecond)
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected a clean quit on the second signal, got %v", err)
		}
	case <-time.After(5 * time.Second):
		program.Kill()
		t.Fatal("second SIGTERM never reached the program")
	}
	if len(quits) != 2 {
		t.Fatalf("expected 2 quit messages, got %d", len(quits))
	}
}

// Cancelling a clone stops the whole process group: git's remote helpers (or
// the git that gh starts) must not outlive the wizard.
func TestInitCloneCancelStopsChildProcesses(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- defaultInitCloneRunner().run(ctx, "sh", []string{"-c", "sleep 30 & echo $! > " + pidFile + "; wait"}, nil)
	}()

	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("child never started")
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancelled run to report context.Canceled, got %v", err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("child %d outlived the cancelled clone", pid)
}
