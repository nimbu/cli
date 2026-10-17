package output

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// spinnerDelay is how long work must run before a live spinner appears, so
// near-instant stages do not flash one on screen.
const spinnerDelay = 150 * time.Millisecond

// CopyTimeline renders a rolling timeline of copy stages to stderr.
// It satisfies migrate.CopyObserver via structural typing.
type CopyTimeline struct {
	mu       sync.Mutex
	writer   io.Writer
	theme    Theme
	glyphs   Glyphs
	animated bool
	dryRun   bool
	started  time.Time
	frame    int
	active   *activeStage
	lines    int // lines used by active stage display (for clearing)

	// spinnerDelay holds back the live display until a stage has run this long.
	spinnerDelay time.Duration

	terminalWidth   int
	pendingWarnings map[string][]string
	warningStages   []string
	lastDoneStage   string
	stopCh          chan struct{}
	doneCh          chan struct{}
	stopOnce        sync.Once
	closed          bool
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

	tl := &CopyTimeline{
		writer:          out,
		theme:           w.ErrTheme(),
		glyphs:          w.Glyphs(),
		animated:        w.ErrAnimated(),
		spinnerDelay:    spinnerDelay,
		dryRun:          dryRun,
		started:         time.Now(),
		pendingWarnings: map[string][]string{},
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
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

	text := fmt.Sprintf("Copying %s %s %s", from, tl.glyphs.Arrow, to)
	if tl.dryRun {
		text = "[dry-run] " + text
	}
	tl.writeLine(tl.colorDim(tl.glyphs.Top) + " " + tl.colorBright(text))
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
	line := tl.colorDone(tl.glyphs.Step) + " " + tl.formatStageLine(name, summary)
	tl.writeLine(line)
	tl.lastDoneStage = name
	tl.flushStageWarningsLocked(name)
}

// StageSkip marks a stage as skipped with a reason.
func (tl *CopyTimeline) StageSkip(name, reason string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.clearActiveLocked()
	tl.active = nil

	tl.writeLine(tl.renderRail())
	text := name + " " + tl.glyphs.Dash + " skipped"
	if reason != "" {
		text += ": " + reason
	}
	tl.writeLine(tl.colorDim(tl.glyphs.Step) + " " + tl.colorDim(text))
	tl.lastDoneStage = name
	tl.flushStageWarningsLocked(name)
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
		tl.writeLine(tl.renderRail() + "  " + tl.colorDim(tl.glyphs.SubStep) + " " + tl.formatStageLine(sub, summary))
	}
}

// StageWarning records or prints a warning owned by one timeline stage.
func (tl *CopyTimeline) StageWarning(stage, msg string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.active != nil && tl.active.name == stage {
		tl.clearActiveLocked()
		tl.queueStageWarningLocked(stage, msg)
		if tl.animated {
			tl.renderActiveLocked()
		}
		return
	}

	tl.clearActiveLocked()
	if stage != "" && stage == tl.lastDoneStage {
		tl.writeWarningLocked(msg)
	} else {
		if stage != "" {
			msg = stage + ": " + msg
		}
		tl.writeWarningLocked(msg)
	}
	if tl.active != nil && tl.animated {
		tl.renderActiveLocked()
	}
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
	tl.flushAllPendingWarningsLocked()
	elapsed := formatElapsed(time.Since(tl.started))

	text := fmt.Sprintf("Done in %s", elapsed)
	if tl.dryRun {
		text = fmt.Sprintf("Dry run complete (%s)", elapsed)
	}

	tl.writeLine(tl.renderRail())
	tl.writeLine(tl.colorDim(tl.glyphs.Bottom) + " " + tl.colorDone("Done!") + "  " + tl.colorDim(text))
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
	tl.flushAllPendingWarningsLocked()

	tl.writeLine(tl.renderRail())
	tl.writeLine(tl.colorDim(tl.glyphs.Bottom) + " " + tl.colorError("Error!") + "  " + tl.colorDim(msg))
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
	tl.flushAllPendingWarningsLocked()
}

// PreparePrompt clears live spinner output before a blocking prompt is printed.
func (tl *CopyTimeline) PreparePrompt() {
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
	if tl.lines == 0 && time.Since(tl.active.started) < tl.spinnerDelay {
		return
	}

	// Clear previous active lines
	tl.eraseActiveLinesLocked()

	// Build the active line: spinner + name + detail
	spinner := tl.colorActive(tl.glyphs.Spinner[tl.frame%len(tl.glyphs.Spinner)])
	tl.frame++

	dash := tl.glyphs.Dash
	stage := tl.active
	var line string
	if stage.activeSub != "" {
		// Nested mode: show sub-stage name instead of parent stage name
		if stage.detail == "" || stage.detail == stage.activeSub {
			line = fmt.Sprintf("%s %s", spinner, stage.activeSub)
		} else if stage.total > 0 {
			line = fmt.Sprintf("%s %s %s %s (%d/%d)", spinner, stage.activeSub, dash, stage.detail, stage.current, stage.total)
		} else {
			line = fmt.Sprintf("%s %s %s %s", spinner, stage.activeSub, dash, stage.detail)
		}
	} else if stage.total > 0 {
		line = fmt.Sprintf("%s %s %s %s (%d/%d)", spinner, stage.name, dash, stage.detail, stage.current, stage.total)
	} else if stage.detail != "" {
		line = fmt.Sprintf("%s %s %s %s", spinner, stage.name, dash, stage.detail)
	} else {
		line = fmt.Sprintf("%s %s", spinner, stage.name)
	}

	// Write connector rail
	tl.writeActiveLineLocked(tl.renderRail())
	n := 1

	// Write completed sub-items
	for _, sub := range stage.subItems {
		tl.writeActiveLineLocked(tl.renderRail() + "  " + tl.colorDim(tl.glyphs.SubStep) + " " + tl.formatStageLine(sub.name, sub.summary))
		n++
	}

	// Write active spinner line (indented if sub-items present)
	if len(stage.subItems) > 0 {
		tl.writeActiveLineLocked(tl.renderRail() + "  " + line)
	} else {
		tl.writeActiveLineLocked(line)
	}
	n++

	tl.lines = n
}

func (tl *CopyTimeline) writeActiveLineLocked(line string) {
	_, _ = fmt.Fprint(tl.writer, tl.fitActiveLine(line)+"\n")
}

func (tl *CopyTimeline) fitActiveLine(line string) string {
	return fitLiveLine(line, liveWidth(tl.terminalWidth, tl.writer), tl.glyphs.Ellipsis)
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
		return tl.colorBright(name)
	}
	return tl.colorBright(name) + " " + tl.colorDim(tl.glyphs.Dash) + " " + summary
}

func (tl *CopyTimeline) renderRail() string {
	return tl.colorDim(tl.glyphs.Rail)
}

func (tl *CopyTimeline) writeLine(line string) {
	_, _ = fmt.Fprintln(tl.writer, line)
}

func (tl *CopyTimeline) queueStageWarningLocked(stage, msg string) {
	if tl.pendingWarnings == nil {
		tl.pendingWarnings = map[string][]string{}
	}
	if len(tl.pendingWarnings[stage]) == 0 {
		tl.warningStages = append(tl.warningStages, stage)
	}
	tl.pendingWarnings[stage] = append(tl.pendingWarnings[stage], msg)
}

func (tl *CopyTimeline) flushStageWarningsLocked(stage string) {
	if len(tl.pendingWarnings) == 0 {
		return
	}
	warnings := tl.pendingWarnings[stage]
	delete(tl.pendingWarnings, stage)
	for _, warning := range warnings {
		tl.writeWarningLocked(warning)
	}
}

func (tl *CopyTimeline) flushAllPendingWarningsLocked() {
	for _, stage := range tl.warningStages {
		warnings := tl.pendingWarnings[stage]
		delete(tl.pendingWarnings, stage)
		for _, warning := range warnings {
			if stage != "" {
				warning = stage + ": " + warning
			}
			tl.writeWarningLocked(warning)
		}
	}
	tl.warningStages = nil
}

func (tl *CopyTimeline) writeWarningLocked(msg string) {
	tl.writeLine(tl.renderRail() + "  " + tl.theme.WarningText(tl.glyphs.Warn+" "+msg))
}

func terminalWidth(w io.Writer) int {
	file, ok := w.(*os.File)
	if !ok {
		return 0
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil || width <= 0 {
		return 0
	}
	return width
}

// Color helpers. Dry runs swap success/active accents for the warning color
// so a rehearsal never looks like a real copy.

func (tl *CopyTimeline) colorDone(s string) string {
	if tl.dryRun {
		return tl.theme.Warning(s)
	}
	return tl.theme.Success(s)
}

func (tl *CopyTimeline) colorActive(s string) string {
	if tl.dryRun {
		return tl.theme.Warning(s)
	}
	return tl.theme.Accent(s)
}

func (tl *CopyTimeline) colorError(s string) string { return tl.theme.Error(s) }

func (tl *CopyTimeline) colorDim(s string) string { return tl.theme.Muted(s) }

func (tl *CopyTimeline) colorBright(s string) string { return tl.theme.Strong(s) }
