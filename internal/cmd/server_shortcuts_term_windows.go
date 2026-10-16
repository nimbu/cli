//go:build windows

package cmd

import (
	"os"

	"golang.org/x/sys/windows"
)

func serverShortcutConsoleMode(mode uint32) uint32 {
	next := mode
	next &^= windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT
	next |= windows.ENABLE_PROCESSED_INPUT
	return next
}

func prepareServerShortcutInput(file *os.File) (serverTermMode, error) {
	handle := windows.Handle(file.Fd())
	var oldMode uint32
	if err := windows.GetConsoleMode(handle, &oldMode); err != nil {
		return serverTermMode{}, err
	}
	nextMode := serverShortcutConsoleMode(oldMode)
	if err := windows.SetConsoleMode(handle, nextMode); err != nil {
		return serverTermMode{}, err
	}
	return serverTermMode{
		apply: func() error {
			return windows.SetConsoleMode(handle, nextMode)
		},
		restore: func() error {
			return windows.SetConsoleMode(handle, oldMode)
		},
	}, nil
}
