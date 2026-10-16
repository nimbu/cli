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
