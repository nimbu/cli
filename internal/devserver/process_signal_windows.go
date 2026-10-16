//go:build windows

package devserver

import (
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// childGroup holds the job object that owns the child's process tree. Node
// dev servers commonly spawn a grandchild that holds the port; killing only
// the direct child would leave it running.
//
// mu guards job: the wait goroutine closes it when the child exits while
// Stop or Kill may be terminating it, and a closed handle value can be
// reused by an unrelated object.
type childGroup struct {
	mu       sync.Mutex
	job      windows.Handle
	released bool
}

// terminate kills every process in the job. It reports true when nothing is
// left to kill: the job was terminated, or it was already released because
// the child exited (closing it reaped the rest). It reports false when there
// is no job or the call failed, so the caller falls back to taskkill.
func (g *childGroup) terminate() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.released {
		// The child exited; its PID may already belong to another process,
		// so a taskkill by PID must not run.
		return true
	}
	if g.job == 0 {
		return false
	}
	return windows.TerminateJobObject(g.job, 1) == nil
}

// configureChildProcess starts the child in its own console process group so
// CTRL_BREAK can be delivered to it (and its descendants) without hitting
// nimbu itself. Process.Signal(os.Interrupt) is not supported on Windows.
func configureChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// attachChildGroup puts the started child in a kill-on-close job object.
// Processes the child spawns afterwards inherit the job. Failure is not fatal:
// killChild falls back to taskkill /T.
func attachChildGroup(cmd *exec.Cmd) *childGroup {
	group := &childGroup{}
	if cmd == nil || cmd.Process == nil {
		return group
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return group
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return group
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return group
	}
	defer func() { _ = windows.CloseHandle(proc) }()
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)
		return group
	}
	group.job = job
	return group
}

// releaseChildGroup runs once the child exited. It closes the job handle, and
// KILL_ON_JOB_CLOSE then reaps any descendant that outlived the child, so
// nothing keeps holding the port. It also marks the group released so a late
// killChild does not taskkill a PID that may have been reused.
func releaseChildGroup(group *childGroup) {
	if group == nil {
		return
	}
	group.mu.Lock()
	defer group.mu.Unlock()
	group.released = true
	if group.job == 0 {
		return
	}
	_ = windows.CloseHandle(group.job)
	group.job = 0
}

// interruptChild asks the child's process group to stop with CTRL_BREAK, the
// console equivalent of SIGINT that can target a single process group.
func interruptChild(cmd *exec.Cmd, _ *childGroup) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
}

// killChild force-kills the whole child tree: the job object when we have
// one, otherwise taskkill /T, and finally the direct child.
func killChild(cmd *exec.Cmd, group *childGroup) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if group.terminate() {
		return nil
	}
	taskkill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	if err := taskkill.Run(); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}
