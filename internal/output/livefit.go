package output

import (
	"io"

	"github.com/charmbracelet/x/ansi"
)

// minLivePathWidth is the narrowest a path in a live line is shortened to
// before the whole line is truncated from the right instead.
const minLivePathWidth = 12

// liveWidth is the width live lines are fitted to: override when set (tests),
// else the terminal width of w, else 0 (unknown: no fitting).
func liveWidth(override int, w io.Writer) int {
	if override > 0 {
		return override
	}
	return terminalWidth(w)
}

// fitLiveLine truncates a live (erasable) line to width-1 cells. Erasing
// moves up one row per line, so a line that wrapped would leave a stale row
// behind on every redraw. Width 0 or 1 means unknown: the line is unchanged.
func fitLiveLine(line string, width int, ellipsis string) string {
	if width <= 1 {
		return line
	}
	maxWidth := width - 1
	if ansi.StringWidth(line) <= maxWidth {
		return line
	}
	return ansi.Truncate(line, maxWidth, ellipsis)
}

// fitLiveTail joins head and tail (a path or label) with sep and fits the
// result to width-1 cells, shortening tail from the left first so the file
// name at its end stays visible.
func fitLiveTail(head, sep, tail string, width int, ellipsis string) string {
	line := head + sep + tail
	if width <= 1 || ansi.StringWidth(line) <= width-1 {
		return line
	}
	budget := width - 1 - ansi.StringWidth(head) - ansi.StringWidth(sep)
	if budget >= minLivePathWidth {
		line = head + sep + truncateLeft(tail, budget, ellipsis)
	}
	return fitLiveLine(line, width, ellipsis)
}

// truncateLeft keeps the last width cells of s, prefixed with ellipsis.
func truncateLeft(s string, width int, ellipsis string) string {
	w := ansi.StringWidth(s)
	if w <= width {
		return s
	}
	keep := width - ansi.StringWidth(ellipsis)
	if keep <= 0 {
		return ansi.Truncate(s, width, "")
	}
	return ellipsis + ansi.TruncateLeft(s, w-keep, "")
}
