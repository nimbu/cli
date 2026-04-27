package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func newAnimatedCopyTimelineForTest(width int, useColor bool) (*CopyTimeline, *bytes.Buffer) {
	var buf bytes.Buffer
	profile := termenv.Ascii
	if useColor {
		profile = termenv.TrueColor
	}
	tl := &CopyTimeline{
		writer:        &buf,
		termOut:       termenv.NewOutput(&buf, termenv.WithProfile(profile)),
		useColor:      useColor,
		animated:      true,
		started:       time.Now(),
		terminalWidth: width,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
	return tl, &buf
}

func newStaticCopyTimelineForTest() (*CopyTimeline, *bytes.Buffer) {
	var buf bytes.Buffer
	tl := &CopyTimeline{
		writer:          &buf,
		termOut:         termenv.NewOutput(&buf, termenv.WithProfile(termenv.Ascii)),
		started:         time.Now(),
		pendingWarnings: map[string][]string{},
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
	}
	return tl, &buf
}

func TestCopyTimelineTruncatesLongActiveLineToTerminalWidth(t *testing.T) {
	tl, buf := newAnimatedCopyTimelineForTest(48, false)
	tl.StageStart("Channel Entries")

	longTitle := strings.Repeat("A very long copied entry title ", 4)
	tl.StageItem("Channel Entries", longTitle, 212, 787)

	lines := nonEscapeOutputLines(buf.String())
	if len(lines) != 2 {
		t.Fatalf("expected rail and active line, got %d lines:\n%s", len(lines), buf.String())
	}
	active := lines[1]
	if !strings.Contains(active, "…") {
		t.Fatalf("expected active line to be truncated with ellipsis, got %q", active)
	}
	assertTerminalWidth(t, active, 47)
}

func TestCopyTimelineTruncatesCompletedSubItemsAndActiveLine(t *testing.T) {
	tl, buf := newAnimatedCopyTimelineForTest(56, false)
	tl.StageStart("Channel Entries")

	tl.StageItem("Channel Entries", "streamingen", 1, 6)
	tl.SubStageDone("Channel Entries", strings.Repeat("streamingen-", 8), "4 synced")
	tl.StageItem("Channel Entries", strings.Repeat("quote title ", 10), 212, 787)

	lines := nonEscapeOutputLines(lastRenderBlock(buf.String()))
	if len(lines) != 3 {
		t.Fatalf("expected rail, sub-item, and active line, got %d lines:\n%s", len(lines), buf.String())
	}
	for _, line := range lines {
		assertTerminalWidth(t, line, 55)
	}
	if !strings.Contains(lines[1], "…") {
		t.Fatalf("expected completed sub-item line to be truncated, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "…") {
		t.Fatalf("expected active line to be truncated, got %q", lines[2])
	}
}

func TestCopyTimelineTruncatesColoredLiveLinesByVisibleWidth(t *testing.T) {
	tl, buf := newAnimatedCopyTimelineForTest(44, true)
	tl.StageStart("Channel Entries")

	tl.StageItem("Channel Entries", strings.Repeat("kleurige titel ", 8), 9, 99)

	lines := nonEscapeOutputLines(buf.String())
	if len(lines) != 2 {
		t.Fatalf("expected rail and active line, got %d lines:\n%s", len(lines), buf.String())
	}
	active := lines[1]
	if !strings.Contains(active, "\x1b[") {
		t.Fatalf("expected colored active line to contain ANSI escapes, got %q", active)
	}
	if !strings.Contains(active, "…") {
		t.Fatalf("expected colored active line to be truncated with ellipsis, got %q", active)
	}
	assertTerminalWidth(t, active, 43)
}

func TestCopyTimelineRendersActiveStageWarningUnderCompletedStage(t *testing.T) {
	tl, buf := newAnimatedCopyTimelineForTest(80, false)
	tl.StageStart("Theme")

	tl.StageWarning("Theme", "skip templates/page.liquid: upload: Validation Failed")
	tl.StageDone("Theme", "106 copied, 1 skipped")

	lines := nonEscapeOutputLines(lastRenderBlock(buf.String()))
	assertLineOrder(t, lines,
		"◇ Theme — 106 copied, 1 skipped",
		"│  ⚠ skip templates/page.liquid: upload: Validation Failed",
	)
}

func TestCopyTimelineRendersLiveStageWarningUnderCompletedStage(t *testing.T) {
	tl, buf := newAnimatedCopyTimelineForTest(80, false)
	tl.StageStart("Theme")
	tl.StageItem("Theme", "templates/page.liquid", 42, 107)

	tl.StageWarning("Theme", "skip templates/page.liquid: upload: Validation Failed")
	tl.StageDone("Theme", "106 copied, 1 skipped")

	lines := nonEscapeOutputLines(lastRenderBlock(buf.String()))
	assertLineOrder(t, lines,
		"◇ Theme — 106 copied, 1 skipped",
		"│  ⚠ skip templates/page.liquid: upload: Validation Failed",
	)
}

func TestCopyTimelineRendersWarningAfterMatchingCompletedStage(t *testing.T) {
	tl, buf := newAnimatedCopyTimelineForTest(80, false)
	tl.StageStart("Theme")
	tl.StageDone("Theme", "106 copied, 1 skipped")

	tl.StageWarning("Theme", "skip templates/page.liquid: upload: Validation Failed")

	lines := nonEscapeOutputLines(lastRenderBlock(buf.String()))
	assertLineOrder(t, lines,
		"◇ Theme — 106 copied, 1 skipped",
		"│  ⚠ skip templates/page.liquid: upload: Validation Failed",
	)
}

func TestCopyTimelineFallbackWarningIncludesStageName(t *testing.T) {
	tl, buf := newAnimatedCopyTimelineForTest(80, false)
	tl.StageStart("Collections")
	tl.StageDone("Collections", "0 synced")

	tl.StageWarning("Theme", "skip templates/page.liquid: upload: Validation Failed")

	lines := nonEscapeOutputLines(lastRenderBlock(buf.String()))
	if !containsLine(lines, "│  ⚠ Theme: skip templates/page.liquid: upload: Validation Failed") {
		t.Fatalf("expected fallback warning to include stage name, got:\n%s", strings.Join(lines, "\n"))
	}
}

func TestCopyTimelineErrorFooterFlushesPendingStageWarning(t *testing.T) {
	tl, buf := newStaticCopyTimelineForTest()
	tl.StageStart("Theme")
	tl.StageWarning("Theme", "skip templates/page.liquid: upload: Validation Failed")

	tl.ErrorFooter("upload failed")

	lines := nonEscapeOutputLines(lastRenderBlock(buf.String()))
	assertLineOrder(t, lines,
		"│  ⚠ Theme: skip templates/page.liquid: upload: Validation Failed",
		"└ Error!  upload failed",
	)
}

func TestCopyTimelineCloseFlushesPendingStageWarning(t *testing.T) {
	tl, buf := newStaticCopyTimelineForTest()
	tl.StageStart("Theme")
	tl.StageWarning("Theme", "skip templates/page.liquid: upload: Validation Failed")

	tl.Close()

	lines := nonEscapeOutputLines(lastRenderBlock(buf.String()))
	if !containsLine(lines, "│  ⚠ Theme: skip templates/page.liquid: upload: Validation Failed") {
		t.Fatalf("expected close to preserve queued warning, got:\n%s", strings.Join(lines, "\n"))
	}
}

func nonEscapeOutputLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(strings.TrimPrefix(line, "\r"), "\r")
		if line == "" || strings.TrimSpace(ansi.Strip(line)) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func lastRenderBlock(output string) string {
	parts := strings.Split(output, "\r")
	return parts[len(parts)-1]
}

func assertTerminalWidth(t *testing.T, line string, maxWidth int) {
	t.Helper()
	if width := ansi.StringWidth(line); width > maxWidth {
		t.Fatalf("line width = %d, want <= %d: %q", width, maxWidth, line)
	}
}

func assertLineOrder(t *testing.T, lines []string, first, second string) {
	t.Helper()
	firstIndex := -1
	secondIndex := -1
	for i, line := range lines {
		switch line {
		case first:
			firstIndex = i
		case second:
			secondIndex = i
		}
	}
	if firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex {
		t.Fatalf("expected %q before %q, got:\n%s", first, second, strings.Join(lines, "\n"))
	}
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}
