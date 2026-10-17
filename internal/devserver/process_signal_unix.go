//go:build !windows

package devserver

import (
	"os/exec"
	"syscall"
	"time"
)

// leftoverGracePeriod is how long processes the child left behind in its
// group get to handle SIGTERM before they are killed.
const leftoverGracePeriod = 500 * time.Millisecond

// childGroup is the process group started with Setpgid; its id is the
// child's pid.
type childGroup struct {
	pgid int
}

// configureChildProcess starts the child in its own process group, so a stop
// can signal everything it started (see releaseChildGroup). The trade-off: a
// signal to nimbu's group no longer reaches the child, so childProcAttr adds
// parent-death handling where the platform has it.
func configureChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = childProcAttr()
}

func attachChildGroup(cmd *exec.Cmd) *childGroup {
	if cmd == nil || cmd.Process == nil {
		return &childGroup{}
	}
	return &childGroup{pgid: cmd.Process.Pid}
}

// releaseChildGroup runs after the child exited and reaps what it left in
// its process group: a server started with `&` in a shell script ignores
// SIGINT (POSIX shells do that for background jobs) and would otherwise keep
// running, and holding the port, after nimbu exits.
func releaseChildGroup(group *childGroup) {
	if group == nil || group.pgid <= 0 {
		return
	}
	if err := syscall.Kill(-group.pgid, syscall.SIGTERM); err != nil {
		return // group is empty
	}
	deadline := time.Now().Add(leftoverGracePeriod)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-group.pgid, 0); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(-group.pgid, syscall.SIGKILL)
}

func interruptChild(cmd *exec.Cmd, _ *childGroup) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
}

func killChild(cmd *exec.Cmd, _ *childGroup) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
