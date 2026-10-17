//go:build windows

package cmd

import "os/exec"

// initCloneProcess keeps exec's default cancel on Windows: the clone process
// is killed when the wizard is cancelled.
func initCloneProcess(cmd *exec.Cmd) {}
