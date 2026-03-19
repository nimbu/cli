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

func prepareServerShortcutInput(file *os.File) (func() error, error) {
	handle := windows.Handle(file.Fd())

	var oldMode uint32
	if err := windows.GetConsoleMode(handle, &oldMode); err != nil {
		return nil, err
	}

	nextMode := serverShortcutConsoleMode(oldMode)
	if err := windows.SetConsoleMode(handle, nextMode); err != nil {
		return nil, err
	}

	return func() error {
		return windows.SetConsoleMode(handle, oldMode)
	}, nil
}
