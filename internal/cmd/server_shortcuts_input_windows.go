//go:build windows

package cmd

import "os"

func openServerShortcutInput() (*os.File, bool, error) {
	return os.Stdin, false, nil
}
