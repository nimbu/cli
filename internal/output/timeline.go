package output

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/muesli/termenv"
)

var brailleSpinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// CopyTimeline renders a rolling timeline of copy stages to stderr.
// It satisfies migrate.CopyObserver via structural typing.
type CopyTimeline struct {
	mu       sync.Mutex
	writer   io.Writer
	termOut  *termenv.Output
	useColor bool
	animated bool
	dryRun   bool
	started  time.Time
	frame    int
	active   *activeStage
	lines    int // lines used by active stage display (for clearing)
	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
	closed   bool
}

type completedSubItem struct {
	name    string
	summary string
}

type activeStage struct {
	name      string
	detail    string
	current   int64
	total     int64
	started   time.Time
	subItems  []completedSubItem
	activeSub string // current sub-stage name (persists across StageItem calls)
}

// NewCopyTimeline creates a timeline renderer. Call Close() when done.
func NewCopyTimeline(ctx context.Context, dryRun bool) *CopyTimeline {
	w := WriterFromContext(ctx)
	out := w.Err
	if out == nil {
		out = w.Out
	}

	profile := termenv.Ascii
	useColor := w.Color == "always" || (w.Color != "never" && w.ErrIsTTY())
	if useColor {
		switch w.Color {
		case "always":
			profile = termenv.TrueColor
		default:
			profile = termenv.EnvColorProfile()
		}
	}

	tl := &CopyTimeline{
		writer:   out,
		termOut:  termenv.NewOutput(out, termenv.WithProfile(profile)),
		useColor: useColor,
		animated: w.ErrIsTTY(),
		dryRun:   dryRun,
		started:  time.Now(),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}

	if tl.animated {
		go tl.loop()
	}

	return tl
}

// Header prints the opening line of the timeline.
func (tl *CopyTimeline) Header(from, to string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	corner := "┌"
	var text string
	if tl.dryRun {
		text = fmt.Sprintf("[dry-run] Copying %s → %s", from, to)
	} else {
		text = fmt.Sprintf("Copying %s → %s", from, to)
	}

	if tl.useColor {
		corner = tl.colorDim(corner)
		text = tl.colorBright(text)
	}

	tl.writeLine(corner + " " + text)
}

// StageStart begins a new active stage with a spinner.
func (tl *CopyTimeline) StageStart(name string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.clearActiveLocked()
	tl.active = &activeStage{
		name:    name,
		started: time.Now(),
	}

	if !tl.animated {
		tl.writeLine(tl.renderRail() + " " + name + "...")
	}
}

// StageItem updates the active stage's sub-detail.
func (tl *CopyTimeline) StageItem(name, detail string, current, total int64) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.active == nil || tl.active.name != name {
		return
	}
	// When sub-items exist and no active sub yet, treat this detail as the sub-stage name
	// (e.g., channel name before per-record progress overwrites detail)
	if len(tl.active.subItems) > 0 && tl.active.activeSub == "" {
		tl.active.activeSub = detail
	}
	tl.active.detail = detail
	tl.active.current = current
	tl.active.total = total

	if !tl.animated {
		return
	}
	tl.renderActiveLocked()
}

// StageDone finalizes a stage with a summary.
func (tl *CopyTimeline) StageDone(name, summary string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.clearActiveLocked()
	tl.active = nil

	// Print connector + done marker
	tl.writeLine(tl.renderRail())
	marker := "◇"
	if tl.useColor {
		marker = tl.colorDone(marker)
	}
	line := marker + " " + tl.formatStageLine(name, summary)
	tl.writeLine(line)
}

// StageSkip marks a stage as skipped with a reason.
func (tl *CopyTimeline) StageSkip(name, reason string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.clearActiveLocked()
	tl.active = nil

	tl.writeLine(tl.renderRail())
	marker := "◇"
	text := name + " — skipped"
	if reason != "" {
		text += ": " + reason
	}
	if tl.useColor {
		marker = tl.colorDim(marker)
		text = tl.colorDim(text)
	}
	tl.writeLine(marker + " " + text)
}

// SubStageDone records a completed sub-item within the active stage.
func (tl *CopyTimeline) SubStageDone(stage, sub, summary string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.active == nil || tl.active.name != stage {
		return
	}
	tl.active.subItems = append(tl.active.subItems, completedSubItem{name: sub, summary: summary})
	tl.active.activeSub = ""
	tl.active.detail = ""
	tl.active.current = 0
	tl.active.total = 0

	if tl.animated {
		tl.renderActiveLocked()
	} else {
		marker := "◦"
		if tl.useColor {
			marker = tl.colorDim(marker)
		}
		tl.writeLine(tl.renderRail() + "  " + marker + " " + tl.formatStageLine(sub, summary))
	}
}

// Warning prints a warning message under the rail.
func (tl *CopyTimeline) Warning(msg string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.clearActiveLocked()

	text := "⚠ " + msg
	if tl.useColor {
		text = tl.termOut.String(text).Foreground(tl.termOut.Color("#f59e0b")).String()
	}
	tl.writeLine(tl.renderRail() + "  " + text)
}

// Footer renders the closing summary line (└ Done! Done in Xs).
// Call this only on success — omit on error so the footer doesn't mislead.
// Idempotent: second call is a no-op.
func (tl *CopyTimeline) Footer() {
	tl.stopLoop()

	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.closed {
		return
	}
	tl.closed = true

	tl.clearActiveLocked()
	elapsed := formatElapsed(time.Since(tl.started))

	corner := "└"
	label := "Done!"
	var text string
	if tl.dryRun {
		text = fmt.Sprintf("Dry run complete (%s)", elapsed)
	} else {
		text = fmt.Sprintf("Done in %s", elapsed)
	}

	if tl.useColor {
		corner = tl.colorDim(corner)
		label = tl.colorDone(label)
		text = tl.colorDim(text)
	}

	tl.writeLine(tl.renderRail())
	tl.writeLine(corner + " " + label + "  " + text)
	tl.writeLine("")
}

// ErrorFooter renders a closing error line (└ Error! <msg>).
// Clears the active spinner, stops the animation loop, and marks closed.
// Idempotent: second call is a no-op.
func (tl *CopyTimeline) ErrorFooter(msg string) {
	tl.stopLoop()

	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.closed {
		return
	}
	tl.closed = true

	tl.clearActiveLocked()

	corner := "└"
	label := "Error!"

	if tl.useColor {
		corner = tl.colorDim(corner)
		label = tl.colorError(label)
		msg = tl.colorDim(msg)
	}

	tl.writeLine(tl.renderRail())
	tl.writeLine(corner + " " + label + "  " + msg)
	tl.writeLine("")
}

// Close stops the spinner goroutine and clears the active stage display.
// Does NOT render the footer — call Footer() first if the operation succeeded.
// Idempotent: safe to call multiple times.
func (tl *CopyTimeline) Close() {
	tl.stopLoop()

	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.clearActiveLocked()
}

func (tl *CopyTimeline) stopLoop() {
	tl.stopOnce.Do(func() {
		if tl.animated {
			close(tl.stopCh)
			<-tl.doneCh
		}
	})
}

func (tl *CopyTimeline) loop() {
	ticker := time.NewTicker(80 * time.Millisecond)
	defer func() {
		ticker.Stop()
		close(tl.doneCh)
	}()

	for {
		select {
		case <-ticker.C:
			tl.mu.Lock()
			if tl.active != nil {
				tl.renderActiveLocked()
			}
			tl.mu.Unlock()
		case <-tl.stopCh:
			return
		}
	}
}

func (tl *CopyTimeline) renderActiveLocked() {
	if tl.active == nil || !tl.animated {
		return
	}

	// Clear previous active lines
	tl.eraseActiveLinesLocked()

	// Build the active line: spinner + name + detail
	spinner := brailleSpinner[tl.frame%len(brailleSpinner)]
	tl.frame++

	if tl.useColor {
		spinner = tl.colorActive(spinner)
	}

	stage := tl.active
	var line string
	if stage.activeSub != "" {
		// Nested mode: show sub-stage name instead of parent stage name
		if stage.detail == "" || stage.detail == stage.activeSub {
			line = fmt.Sprintf("%s %s", spinner, stage.activeSub)
		} else if stage.total > 0 {
			line = fmt.Sprintf("%s %s — %s (%d/%d)", spinner, stage.activeSub, stage.detail, stage.current, stage.total)
		} else {
			line = fmt.Sprintf("%s %s — %s", spinner, stage.activeSub, stage.detail)
		}
	} else if stage.total > 0 {
		line = fmt.Sprintf("%s %s — %s (%d/%d)", spinner, stage.name, stage.detail, stage.current, stage.total)
	} else if stage.detail != "" {
		line = fmt.Sprintf("%s %s — %s", spinner, stage.name, stage.detail)
	} else {
		line = fmt.Sprintf("%s %s", spinner, stage.name)
	}

	// Write connector rail
	_, _ = fmt.Fprint(tl.writer, tl.renderRail()+"\n")
	n := 1

	// Write completed sub-items
	for _, sub := range stage.subItems {
		marker := "◦"
		if tl.useColor {
			marker = tl.colorDim(marker)
		}
		_, _ = fmt.Fprint(tl.writer, tl.renderRail()+"  "+marker+" "+tl.formatStageLine(sub.name, sub.summary)+"\n")
		n++
	}

	// Write active spinner line (indented if sub-items present)
	if len(stage.subItems) > 0 {
		_, _ = fmt.Fprint(tl.writer, tl.renderRail()+"  "+line+"\n")
	} else {
		_, _ = fmt.Fprint(tl.writer, line+"\n")
	}
	n++

	tl.lines = n
}

func (tl *CopyTimeline) clearActiveLocked() {
	if !tl.animated || tl.lines == 0 {
		return
	}
	tl.eraseActiveLinesLocked()
	tl.lines = 0
}

func (tl *CopyTimeline) eraseActiveLinesLocked() {
	for i := 0; i < tl.lines; i++ {
		// Move up one line and clear it
		_, _ = fmt.Fprint(tl.writer, "\033[1A\033[2K")
	}
	_, _ = fmt.Fprint(tl.writer, "\r")
}

func (tl *CopyTimeline) formatStageLine(name, summary string) string {
	if summary == "" {
		if tl.useColor {
			return tl.colorBright(name)
		}
		return name
	}
	if tl.useColor {
		return tl.colorBright(name) + " " + tl.colorDim("—") + " " + summary
	}
	return name + " — " + summary
}

func (tl *CopyTimeline) renderRail() string {
	rail := "│"
	if tl.useColor {
		rail = tl.colorDim(rail)
	}
	return rail
}

func (tl *CopyTimeline) writeLine(line string) {
	_, _ = fmt.Fprintln(tl.writer, line)
}

// Color helpers

func (tl *CopyTimeline) colorDone(s string) string {
	color := "#22c55e" // green
	if tl.dryRun {
		color = "#f59e0b" // amber
	}
	return tl.termOut.String(s).Foreground(tl.termOut.Color(color)).Bold().String()
}

func (tl *CopyTimeline) colorActive(s string) string {
	color := "#60a5fa" // blue
	if tl.dryRun {
		color = "#f59e0b" // amber
	}
	return tl.termOut.String(s).Foreground(tl.termOut.Color(color)).Bold().String()
}

func (tl *CopyTimeline) colorError(s string) string {
	return tl.termOut.String(s).Foreground(tl.termOut.Color("#ef4444")).Bold().String()
}

func (tl *CopyTimeline) colorDim(s string) string {
	return tl.termOut.String(s).Foreground(tl.termOut.Color("#94a3b8")).String()
}

func (tl *CopyTimeline) colorBright(s string) string {
	return tl.termOut.String(s).Foreground(tl.termOut.Color("#e2e8f0")).Bold().String()
}
