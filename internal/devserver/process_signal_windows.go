//go:build windows

package devserver

import (
	"os"
	"os/exec"
)

func configureChildProcess(_ *exec.Cmd) {}

func interruptChild(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}

func killChild(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
