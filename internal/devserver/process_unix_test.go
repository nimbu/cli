//go:build !windows

package devserver

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestProcessRoutesOutputToConfiguredWriters(t *testing.T) {
	var stdout, stderr lockedBuffer
	p := NewProcess(ChildConfig{
		Command: "sh",
		Args:    []string{"-c", "echo out; echo err 1>&2"},
		Stdout:  &stdout,
		Stderr:  &stderr,
	})
	if err := p.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-p.ExitCh():
	case <-time.After(5 * time.Second):
		t.Fatal("child did not exit")
	}
	if got := stdout.String(); got != "out\n" {
		t.Fatalf("stdout = %q", got)
	}
	if got := stderr.String(); got != "err\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestProcessStopKillsChildThatIgnoresInterrupt(t *testing.T) {
	p := NewProcess(ChildConfig{
		Command: "sh",
		Args:    []string{"-c", `trap "" INT; while :; do sleep 1; done`},
	})
	if err := p.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	if err := p.Stop(200 * time.Millisecond); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("stop took %s", elapsed)
	}
}

func TestProcessKillTakesWholeProcessGroup(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "grandchild.pid")
	p := NewProcess(ChildConfig{
		Command: "sh",
		Args:    []string{"-c", `trap "" INT; sleep 60 & echo $! > "$0"; wait`, pidPath},
	})
	if err := p.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	grandchild := waitForPID(t, pidPath)
	start := time.Now()
	if err := p.Kill(5 * time.Second); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("kill took %s; expected immediate", elapsed)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(grandchild, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(grandchild, syscall.SIGKILL)
	t.Fatalf("grandchild %d survived Kill", grandchild)
}

func TestProcessStopReapsLeftoverGroupMembers(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "background.pid")
	// The backgrounded sleep ignores SIGINT (shells do that for & jobs), so
	// the graceful interrupt only ends the shell itself.
	p := NewProcess(ChildConfig{
		Command: "sh",
		Args:    []string{"-c", `sleep 60 & echo $! > "$0"; wait`, pidPath},
	})
	if err := p.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	background := waitForPID(t, pidPath)

	if err := p.Stop(5 * time.Second); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitForProcessGone(t, background)
}

func TestProcessStopIsNotHeldUpByLeftoverHoldingOutputPipe(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "background.pid")
	var stdout lockedBuffer
	// The backgrounded sleep inherits the stdout pipe and ignores SIGINT.
	p := NewProcess(ChildConfig{
		Command: "sh",
		Args:    []string{"-c", `sleep 60 & echo $! > "$0"; echo started; wait`, pidPath},
		Stdout:  &stdout,
	})
	if err := p.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	background := waitForPID(t, pidPath)

	start := time.Now()
	if err := p.Stop(5 * time.Second); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("stop took %s; the open pipe should not delay it", elapsed)
	}
	waitForProcessGone(t, background)
	if got := stdout.String(); got != "started\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func waitForProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("process %d survived", pid)
}

func TestProcessKillWhenNotRunningIsNoop(t *testing.T) {
	p := NewProcess(ChildConfig{Command: "sh"})
	if err := p.Kill(time.Second); err != nil {
		t.Fatalf("kill: %v", err)
	}
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("pid file %s not written", path)
	return 0
}

// Once the direct child exits, the wait goroutine still reaps what it left in
// its group. Stop and Kill arriving then must wait for that reaping, or a
// second Ctrl-C exits nimbu with a leftover still holding the port.
func TestProcessStopAndKillWaitForLeftoverReaping(t *testing.T) {
	for _, tc := range []struct {
		name string
		stop func(*Process) error
	}{
		{"stop", func(p *Process) error { return p.Stop(5 * time.Second) }},
		{"kill", func(p *Process) error { return p.Kill(5 * time.Second) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pidPath := filepath.Join(t.TempDir(), "leftover.pid")
			// The leftover ignores SIGTERM, so only the SIGKILL that follows
			// leftoverGracePeriod ends it; the shell exits at once.
			p := NewProcess(ChildConfig{
				Command: "sh",
				Args:    []string{"-c", `(trap "" TERM; exec sleep 60) & echo $! > "$0"`, pidPath},
			})
			if err := p.Start(); err != nil {
				t.Fatalf("start: %v", err)
			}
			leftover := waitForPID(t, pidPath)
			t.Cleanup(func() { _ = syscall.Kill(leftover, syscall.SIGKILL) })
			time.Sleep(100 * time.Millisecond) // the shell has exited; reaping is under way

			if err := tc.stop(p); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			deadline := time.Now().Add(100 * time.Millisecond)
			for time.Now().Before(deadline) {
				if err := syscall.Kill(leftover, 0); errors.Is(err, syscall.ESRCH) {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatalf("%s returned while leftover %d was still running", tc.name, leftover)
		})
	}
}
