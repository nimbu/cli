//go:build !windows

package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nimbu/cli/internal/output"
)

// withTestRootInterrupt installs the real root handler (real signal.Notify) as
// Execute does, with forceExit stubbed. A guard subscription keeps a stray
// signal from killing the test binary once the handler is stopped.
func withTestRootInterrupt(t *testing.T, parent context.Context) (context.Context, <-chan int) {
	t.Helper()
	guard := make(chan os.Signal, 8)
	signal.Notify(guard, os.Interrupt, syscall.SIGTERM)
	codes := stubForceExit(t)
	ctx, stop := withInterrupt(parent)
	t.Cleanup(func() {
		stop()
		signal.Stop(guard)
	})
	return ctx, codes
}

func sendSelf(t *testing.T, sig syscall.Signal) {
	t.Helper()
	if err := syscall.Kill(os.Getpid(), sig); err != nil {
		t.Fatalf("send %v: %v", sig, err)
	}
}

func assertNoForceExit(t *testing.T, codes <-chan int) {
	t.Helper()
	select {
	case code := <-codes:
		t.Fatalf("root force-exited %d", code)
	default:
	}
}

// G2: apps logs --tail stops on the root context. One Ctrl-C ends the tail
// with exit 0 and is not counted twice.
func TestAppsLogsTailEndsOnOneRootInterrupt(t *testing.T) {
	originalPollInterval := appsLogsPollInterval
	appsLogsPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { appsLogsPollInterval = originalPollInterval })

	polled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case polled <- struct{}{}:
		default:
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	rootCtx, codes := withTestRootInterrupt(t, context.Background())
	ctx, _, _ := newAppsLogsTestContextFromBase(t, rootCtx, server.URL, output.Mode{})
	cmd := &AppsLogsCmd{App: "storefront", Limit: 2, Tail: true}
	errCh := make(chan error, 1)
	go func() { errCh <- cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL}) }()

	select {
	case <-polled:
	case err := <-errCh:
		t.Fatalf("tail ended before polling: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("tail never polled")
	}
	sendSelf(t, syscall.SIGINT)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("tail after Ctrl-C = %v, want a clean stop", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("one Ctrl-C did not stop the tail")
	}
	if !interrupted(rootCtx) {
		t.Fatal("root context should carry the interrupt")
	}
	time.Sleep(100 * time.Millisecond)
	assertNoForceExit(t, codes)
}

// G4: while a Bubble Tea program runs, it owns SIGINT/SIGTERM. Under the root
// handler each signal reaches the program exactly once, the root context the
// program runs under stays alive (canceling it would kill the program past
// the init wizard's hold filter), and the root never force-exits.
func TestProgramSignalsUnderRootHandler(t *testing.T) {
	rootCtx, codes := withTestRootInterrupt(t, context.Background())

	quits := make(chan struct{}, 4)
	filter := func(_ tea.Model, msg tea.Msg) tea.Msg {
		if _, ok := msg.(tea.QuitMsg); ok {
			quits <- struct{}{}
			if len(quits) < 2 {
				return nil // held, like the wizard while it writes files
			}
		}
		return msg
	}
	program := tea.NewProgram(initSignalCountModel{},
		tea.WithContext(rootCtx),
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
		tea.WithoutRenderer(),
		tea.WithoutSignalHandler(),
		tea.WithFilter(filter),
	)
	stop := forwardProgramSignals(program)
	defer stop()

	done := make(chan error, 1)
	go func() {
		_, err := program.Run()
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	sendSelf(t, syscall.SIGTERM)
	time.Sleep(100 * time.Millisecond)
	if rootCtx.Err() != nil {
		t.Fatal("root canceled the program's context on a claimed signal")
	}
	sendSelf(t, syscall.SIGTERM)
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
	assertNoForceExit(t, codes)
}

// G3: a forced exit while a program runs kills it first, so Bubble Tea
// restores the terminal (raw mode, alternate screen) before nimbu exits.
func TestForcedExitKillsRunningProgram(t *testing.T) {
	program := tea.NewProgram(initSignalCountModel{},
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
		tea.WithoutRenderer(),
		tea.WithoutSignalHandler(),
	)
	stop := forwardProgramSignals(program)
	defer stop()

	done := make(chan error, 1)
	go func() {
		_, err := program.Run()
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)

	runTerminalRestores(2 * time.Second)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected the program to be killed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("forced-exit restores did not stop the program")
	}
}
