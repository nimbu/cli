//go:build windows

package cmd

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestServerShortcutConsoleModePreservesProcessedInput(t *testing.T) {
	initial := uint32(
		windows.ENABLE_ECHO_INPUT |
			windows.ENABLE_LINE_INPUT |
			windows.ENABLE_PROCESSED_INPUT,
	)

	got := serverShortcutConsoleMode(initial)

	if got&windows.ENABLE_ECHO_INPUT != 0 {
		t.Fatal("expected ENABLE_ECHO_INPUT to be disabled")
	}
	if got&windows.ENABLE_LINE_INPUT != 0 {
		t.Fatal("expected ENABLE_LINE_INPUT to be disabled")
	}
	if got&windows.ENABLE_PROCESSED_INPUT == 0 {
		t.Fatal("expected ENABLE_PROCESSED_INPUT to remain enabled")
	}
}
