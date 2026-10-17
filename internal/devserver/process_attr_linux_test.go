//go:build linux

package devserver

import (
	"os/exec"
	"syscall"
	"testing"
)

// The child runs in its own process group, so a SIGKILL to nimbu's group no
// longer reaches it; the parent-death signal must take over.
func TestConfigureChildProcessSetsParentDeathSignal(t *testing.T) {
	cmd := exec.Command("true")
	configureChildProcess(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("child must run in its own process group")
	}
	if cmd.SysProcAttr.Pdeathsig != syscall.SIGTERM {
		t.Fatalf("Pdeathsig = %v, want SIGTERM", cmd.SysProcAttr.Pdeathsig)
	}
}
