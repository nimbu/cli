package cmd

import (
	"errors"
	"testing"
	"time"
)

func TestServerStopBeginsOnceThenAsksToForce(t *testing.T) {
	release := make(chan struct{})
	stop := &serverStop{graceful: func() { <-release }}

	if stop.Stopping() {
		t.Fatal("not stopping before Begin")
	}
	if stop.Done() != nil {
		t.Fatal("Done must be nil (block) before Begin")
	}
	if !stop.Begin(nil) {
		t.Fatal("first Begin should start the graceful stop")
	}
	if !stop.Stopping() {
		t.Fatal("stopping after Begin")
	}
	if stop.Begin(nil) {
		t.Fatal("second Begin must report that a stop is already running")
	}

	select {
	case <-stop.Done():
		t.Fatal("done before the graceful stop finished")
	default:
	}
	close(release)
	select {
	case <-stop.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done not closed after graceful stop")
	}
}

func TestServerForceStopErrorExitsWith130Silently(t *testing.T) {
	var exitErr *ExitError
	if !errors.As(errServerForceStopped, &exitErr) || exitErr.Code != 130 {
		t.Fatalf("force stop exit = %v, want 130", errServerForceStopped)
	}
	var displayed *displayedError
	if !errors.As(errServerForceStopped, &displayed) {
		t.Fatal("force stop error must be marked displayed so it is not printed twice")
	}
	if desc := classifyError(errServerForceStopped); desc.ExitCode != 130 {
		t.Fatalf("classified exit code = %d, want 130", desc.ExitCode)
	}
}
