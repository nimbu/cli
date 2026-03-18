//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || zos

package cmd

import (
	"os"

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

func prepareServerShortcutInput(file *os.File) (func() error, error) {
	fd := int(file.Fd())

	termios, err := unix.IoctlGetTermios(fd, serverShortcutReadTermios)
	if err != nil {
		return nil, err
	}

	oldState := *termios
	nextState := serverShortcutTermios(oldState)
	if err := unix.IoctlSetTermios(fd, serverShortcutWriteTermios, &nextState); err != nil {
		return nil, err
	}

	return func() error {
		return unix.IoctlSetTermios(fd, serverShortcutWriteTermios, &oldState)
	}, nil
}
