//go:build !windows && !linux

package devserver

import "syscall"

// childProcAttr has no parent-death signal here (macOS and the BSDs lack
// one): if nimbu is SIGKILLed the child keeps running in its own group. The
// README says how to clean it up.
func childProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
