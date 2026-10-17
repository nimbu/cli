package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

const longThemePath = "templates/customers/account/very/deeply/nested/folder/structure/with/a_really_long_template_name.liquid"

func TestSyncTimelineFitsLongActiveFileToTerminalWidth(t *testing.T) {
	var buf bytes.Buffer
	tl := &SyncTimeline{
		writer:        &buf,
		glyphs:        GlyphSet(false),
		animated:      true,
		activeCat:     0,
		activeFile:    longThemePath,
		terminalWidth: 80,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
	tl.cats = []catState{{label: "Templates", total: 5}}
	tl.padWidth = len("Templates")

	tl.mu.Lock()
	for i := 0; i < 3; i++ {
		tl.renderActiveLocked()
	}
	tl.mu.Unlock()

	lines := nonEscapeOutputLines(buf.String())
	if len(lines) != 6 {
		t.Fatalf("expected 3 redraws of rail + active line, got %d lines:\n%q", len(lines), buf.String())
	}
	for _, line := range lines {
		assertTerminalWidth(t, ansi.Strip(line), 79)
	}
	active := lines[len(lines)-1]
	if !strings.HasSuffix(active, "a_really_long_template_name.liquid") || !strings.Contains(active, "…") {
		t.Fatalf("expected the path shortened from the left, got %q", active)
	}
	if !strings.Contains(active, "Templates") || !strings.Contains(active, "0/5") {
		t.Fatalf("expected the category and counter to survive, got %q", active)
	}
}

func TestProgressFitsLongLiveLabelToTerminalWidth(t *testing.T) {
	p, buf := newAnimatedProgressForTest(t, time.Hour)
	p.mu.Lock()
	p.terminalWidth = 60
	p.mu.Unlock()

	task := p.Transfer("download "+longThemePath, 2048)
	task.Add(1024)
	p.mu.Lock()
	p.spinnerDelay = 0
	p.renderLocked(false)
	p.renderLocked(false)
	p.mu.Unlock()

	for _, frame := range strings.Split(buf.String(), clearLine) {
		if frame == "" {
			continue
		}
		assertTerminalWidth(t, ansi.Strip(frame), 59)
		if !strings.Contains(frame, "name.liquid") {
			t.Fatalf("expected the label's file name to survive, got %q", frame)
		}
	}

	task.Done("done")
	if out := buf.String(); !strings.Contains(out, "download "+longThemePath) {
		t.Fatalf("the final line keeps the full label, got %q", out)
	}
}

func TestTruncateLeftKeepsTail(t *testing.T) {
	if got := truncateLeft("abcdefghij", 6, "…"); got != "…fghij" {
		t.Fatalf("truncateLeft = %q", got)
	}
	if got := truncateLeft("abc", 6, "…"); got != "abc" {
		t.Fatalf("short input changed: %q", got)
	}
	if got := fitLiveLine("abc", 0, "…"); got != "abc" {
		t.Fatalf("unknown width must not truncate: %q", got)
	}
}
