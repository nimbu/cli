//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || zos

package cmd

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// Keep output processing and signal handling intact; we only need single-byte,
// no-echo input for shortcuts.
func serverShortcutTermios(termios unix.Termios) unix.Termios {
	next := termios
	next.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON
	next.Cc[unix.VMIN] = 1
	next.Cc[unix.VTIME] = 0
	return next
}

// prepareServerShortcutInput switches file to shortcut mode and returns the
// pair that re-applies and restores it (used across Ctrl-Z / fg).
func prepareServerShortcutInput(file *os.File) (serverTermMode, error) {
	fd := int(file.Fd())
	termios, err := unix.IoctlGetTermios(fd, serverShortcutReadTermios)
	if err != nil {
		return serverTermMode{}, err
	}
	oldState := *termios
	nextState := serverShortcutTermios(oldState)
	mode := serverTermMode{
		// Setting termios from a background job (resumed with `bg`, or
		// started with `&`) raises SIGTTOU and would stop nimbu again; only
		// the foreground job may. In the background the terminal is already
		// in the shell's mode: Ctrl-Z restored it before `bg`.
		apply: func() error {
			if !serverTerminalIsForeground(fd) {
				return nil
			}
			return unix.IoctlSetTermios(fd, serverShortcutWriteTermios, &nextState)
		},
		restore: func() error {
			if !serverTerminalIsForeground(fd) {
				return nil
			}
			return unix.IoctlSetTermios(fd, serverShortcutWriteTermios, &oldState)
		},
		foreground: func() bool { return serverTerminalIsForeground(fd) },
	}
	if err := mode.apply(); err != nil {
		return serverTermMode{}, err
	}
	return mode, nil
}

func serverTerminalIsForeground(fd int) bool {
	pgrp, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		return false
	}
	return pgrp == syscall.Getpgrp()
}
