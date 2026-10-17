//go:build !windows

package cmd

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// initCloneProcess runs a clone in its own process group and, when the clone
// is cancelled, sends SIGTERM to the whole group. Killing only the leader
// orphans its children: git's remote helpers (git-remote-https, ssh) or the
// git that `gh repo clone` starts would keep running after the wizard exits.
// SIGTERM lets git remove its partial clone; WaitDelay escalates to SIGKILL.
//
// The child never reads the terminal (stdin is /dev/null and every prompt is
// disabled by initCloneEnv), so leaving the foreground group is safe.
func initCloneProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
