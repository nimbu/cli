package cmd

import (
	"runtime"
	"testing"
)

// termenv ignores TERM/COLORTERM on Windows and derives the color depth from
// the OS build, so a pinned 256-color terminal cannot be simulated there.
func skipColorDepthOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("termenv picks color depth from the Windows build, not TERM")
	}
}
