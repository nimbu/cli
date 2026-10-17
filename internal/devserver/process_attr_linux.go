//go:build linux

package devserver

import "syscall"

// childProcAttr also asks the kernel to SIGTERM the child when nimbu dies
// without stopping it (SIGKILL, a crash), which a kill of nimbu's process group
// no longer covers. SIGTERM rather than SIGKILL lets a runner such as npm
// stop the processes it started. Pdeathsig fires when the thread that started
// the child exits; Go keeps its threads for the life of the process unless a
// goroutine exits while locked to one, which nimbu never does.
func childProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
}
