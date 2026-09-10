package output

import (
	"strings"

	"github.com/aymanbagabas/go-udiff"
)

// Unified renders a git-style unified diff. Line endings are normalized and a
// trailing newline is enforced so callers can compare text leniently.
func Unified(fromLabel, toLabel, from, to string) string {
	return udiff.Unified(fromLabel, toLabel, unifiedDiffContent(from), unifiedDiffContent(to))
}

func unifiedDiffContent(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if value != "" && !strings.HasSuffix(value, "\n") {
		value += "\n"
	}
	return value
}
