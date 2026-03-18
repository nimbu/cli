//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package cmd

import "golang.org/x/sys/unix"

const (
	serverShortcutReadTermios  = unix.TIOCGETA
	serverShortcutWriteTermios = unix.TIOCSETA
)
