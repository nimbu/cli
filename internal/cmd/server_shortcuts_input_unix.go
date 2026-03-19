//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || zos

package cmd

import "os"

func openServerShortcutInput() (*os.File, bool, error) {
	input, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err == nil {
		return input, true, nil
	}
	return os.Stdin, false, nil
}
