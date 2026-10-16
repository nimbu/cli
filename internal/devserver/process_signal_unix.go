//go:build !windows

package devserver

import (
	"os/exec"
	"syscall"
)

// childGroup is the process group started with Setpgid; on unix the group id
// equals the child's pid, so no extra state is needed.
type childGroup struct{}

func configureChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachChildGroup(*exec.Cmd) *childGroup { return &childGroup{} }

func releaseChildGroup(*childGroup) {}

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
