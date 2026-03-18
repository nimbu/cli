//go:build aix || linux || solaris || zos

package cmd

import "golang.org/x/sys/unix"

const (
	serverShortcutReadTermios  = unix.TCGETS
	serverShortcutWriteTermios = unix.TCSETS
)
