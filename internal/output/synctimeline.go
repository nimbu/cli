package output

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// SyncTimeline renders category-grouped progress for theme push/sync.
type SyncTimeline struct {
	mu         sync.Mutex
	writer     io.Writer
	theme      Theme
	glyphs     Glyphs
	animated   bool
	dryRun     bool
	mode       string // "push" or "sync"
	themeName  string
	started    time.Time
	frame      int
	padWidth   int // longest category label width for alignment
	cats       []catState
	activeCat  int // index into cats, -1 when idle
	activeFile string
	// activeSince marks when the current category or delete phase began;
	// the live spinner waits spinnerDelay before it first appears.
	activeSince  time.Time
	spinnerDelay time.Duration
	delState     *deleteState
	lines        int // volatile lines for ANSI erase
	// terminalWidth overrides the detected width live lines are fitted to
	// (tests); 0 detects it from the writer.
	terminalWidth int
	stopCh        chan struct{}
	doneCh        chan struct{}
	stopOnce      sync.Once
	closed        bool
	loopOnce      sync.Once
	uploaded      int
	deleted       int
	failed        int
	skipped       int
}

type catState struct {
	label    string
	total    int
	done     int
	skipped  int
	finished bool
}

type deleteState struct {
	total int
	done  int
}

type syncTimelineCtxKey struct{}

// WithSyncTimeline stores a SyncTimeline in context.
func WithSyncTimeline(ctx context.Context, tl *SyncTimeline) context.Context {
	return context.WithValue(ctx, syncTimelineCtxKey{}, tl)
}

// SyncTimelineFromContext retrieves the SyncTimeline from context, or nil.
func SyncTimelineFromContext(ctx context.Context) *SyncTimeline {
	tl, _ := ctx.Value(syncTimelineCtxKey{}).(*SyncTimeline)
	return tl
}

// SyncCategory describes one category for the timeline.
type SyncCategory struct {
	Label string
	Count int
}

// NewSyncTimeline creates a timeline renderer for theme push/sync.
func NewSyncTimeline(ctx context.Context, mode, theme string, dryRun bool) *SyncTimeline {
	w := WriterFromContext(ctx)
	out := w.Err
	if out == nil {
		out = w.Out
	}

	return &SyncTimeline{
		writer:       out,
		theme:        w.ErrTheme(),
		glyphs:       w.Glyphs(),
		animated:     w.ErrAnimated(),
		spinnerDelay: spinnerDelay,
		dryRun:       dryRun,
		mode:         mode,
		themeName:    theme,
		activeCat:    -1,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// SetCategories initializes the category list and computes label alignment.
func (tl *SyncTimeline) SetCategories(categories []SyncCategory) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.cats = make([]catState, 0, len(categories))
	maxLen := 0
	for _, c := range categories {
		if len(c.Label) > maxLen {
			maxLen = len(c.Label)
		}
		tl.cats = append(tl.cats, catState{label: c.Label, total: c.Count})
	}
	tl.padWidth = maxLen
}

// Header prints the opening line and starts the animation loop.
func (tl *SyncTimeline) Header() {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.started = time.Now()
	if tl.animated {
		tl.loopOnce.Do(func() { go tl.loop() })
	}

	if tl.animated {
		text := fmt.Sprintf(`%s theme "%s"`, tl.gerund(), tl.themeName)
		if tl.dryRun {
			text += "  [dry-run]"
		}
		tl.writeLine(tl.colorDim(tl.glyphs.Top) + " " + tl.colorBright(text))
	} else {
		text := fmt.Sprintf(`%s theme "%s"`, tl.mode, tl.themeName)
		if tl.dryRun {
			text += " [dry-run]"
		}
		tl.writeLine(text)
	}
}

// StartCategory begins a new active category with a spinner.
func (tl *SyncTimeline) StartCategory(index int) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.clearActiveLocked()
	if index >= 0 && index < len(tl.cats) {
		tl.activeCat = index
	}
	tl.activeFile = ""
	tl.activeSince = time.Now()
}

// SetActiveFile updates the spinner to show the file currently being uploaded.
func (tl *SyncTimeline) SetActiveFile(displayPath string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.activeFile = displayPath
	if tl.animated {
		tl.renderActiveLocked()
	}
}

// FileUploaded records a successful upload and increments the category counter.
func (tl *SyncTimeline) FileUploaded() {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if tl.activeCat >= 0 && tl.activeCat < len(tl.cats) {
		tl.cats[tl.activeCat].done++
	}
	tl.uploaded++
}

// FileSkipped records an intentionally skipped upload after user confirmation.
func (tl *SyncTimeline) FileSkipped() {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.clearActiveLocked()
	if tl.activeCat >= 0 && tl.activeCat < len(tl.cats) {
		tl.cats[tl.activeCat].skipped++
	}
	tl.skipped++
	tl.activeFile = ""
}

// CategoryDone finalizes a category, writing a permanent completion line.
func (tl *SyncTimeline) CategoryDone(index int) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.clearActiveLocked()
	if index < 0 || index >= len(tl.cats) {
		return
	}

	tl.cats[index].finished = true
	tl.activeCat = -1
	tl.activeFile = ""
	cat := tl.cats[index]
	summary := fmt.Sprintf("%d %s", cat.done, tl.pastVerb())
	if cat.skipped > 0 {
		summary = fmt.Sprintf("%d %s, %d skipped", cat.done, tl.pastVerb(), cat.skipped)
	}

	if tl.animated {
		tl.writeLine(tl.renderRail())
		tl.writeLine(tl.colorDone(tl.glyphs.Step) + " " + tl.formatCatLine(cat.label, summary))
	} else {
		tl.writeLine(fmt.Sprintf("  %s: %s", cat.label, summary))
	}
}

// StartDeletes begins the delete phase with a spinner.
func (tl *SyncTimeline) StartDeletes(total int) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.clearActiveLocked()
	tl.activeCat = -1
	tl.delState = &deleteState{total: total}
	tl.activeFile = ""
	tl.activeSince = time.Now()
}

// SetActiveDelete updates the spinner to show the file being deleted.
func (tl *SyncTimeline) SetActiveDelete(displayPath string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.activeFile = displayPath
	if tl.animated {
		tl.renderActiveLocked()
	}
}

// FileDeleted records a successful delete.
func (tl *SyncTimeline) FileDeleted() {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if tl.delState != nil {
		tl.delState.done++
	}
	tl.deleted++
}

// DeletesDone finalizes the delete phase with a permanent completion line.
func (tl *SyncTimeline) DeletesDone() {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.clearActiveLocked()
	if tl.delState == nil {
		return
	}

	summary := fmt.Sprintf("%d %s", tl.delState.done, plural(tl.delState.done, "file", "files"))
	if tl.animated {
		tl.writeLine(tl.renderRail())
		tl.writeLine(tl.colorDone(tl.glyphs.Step) + " " + tl.formatCatLine("Deleted", summary))
	} else {
		tl.writeLine(fmt.Sprintf("  deleted: %s", summary))
	}
	tl.delState = nil
}

// FileFailed records an upload failure. errorDetail is a pre-formatted string.
func (tl *SyncTimeline) FileFailed(displayPath, errorDetail string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.clearActiveLocked()
	tl.failed++

	if tl.activeCat >= 0 && tl.activeCat < len(tl.cats) {
		cat := tl.cats[tl.activeCat]
		tl.skipped = cat.total - cat.done
		for i := tl.activeCat + 1; i < len(tl.cats); i++ {
			if !tl.cats[i].finished {
				tl.skipped += tl.cats[i].total
			}
		}

		if tl.animated {
			tl.writeLine(tl.renderRail())
			marker := tl.colorError(tl.glyphs.Fail)
			text := fmt.Sprintf("failed at %s (%d/%d)", displayPath, cat.done, cat.total)
			tl.writeLine(marker + " " + tl.formatCatLine(cat.label, text))
			if errorDetail != "" {
				tl.writeLine(tl.renderRail() + "  " + errorDetail)
			}
		} else {
			tl.writeLine(fmt.Sprintf("  %s: failed at %s (%d/%d)", cat.label, displayPath, cat.done, cat.total))
			if errorDetail != "" {
				tl.writeLine(fmt.Sprintf("    %s", errorDetail))
			}
		}
	}

	tl.activeCat = -1
	tl.activeFile = ""
}

// DeleteFailed records a delete failure.
func (tl *SyncTimeline) DeleteFailed(displayPath, errorDetail string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.clearActiveLocked()
	tl.failed++
	if tl.delState != nil {
		tl.skipped = tl.delState.total - tl.delState.done
	}

	if tl.animated {
		tl.writeLine(tl.renderRail())
		marker := tl.colorError(tl.glyphs.Fail)
		var text string
		if tl.delState != nil {
			text = fmt.Sprintf("failed at %s (%d/%d)", displayPath, tl.delState.done, tl.delState.total)
		} else {
			text = fmt.Sprintf("failed at %s", displayPath)
		}
		tl.writeLine(marker + " " + tl.formatCatLine("Deleting", text))
		if errorDetail != "" {
			tl.writeLine(tl.renderRail() + "  " + errorDetail)
		}
	} else {
		tl.writeLine(fmt.Sprintf("  deleting: failed at %s", displayPath))
		if errorDetail != "" {
			tl.writeLine(fmt.Sprintf("    %s", errorDetail))
		}
	}

	tl.delState = nil
	tl.activeFile = ""
}

// RenderPlan displays the dry-run plan without executing uploads.
func (tl *SyncTimeline) RenderPlan(categories []SyncCategory, deleteCount int) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.started = time.Now()

	maxLen := 0
	for _, c := range categories {
		if len(c.Label) > maxLen {
			maxLen = len(c.Label)
		}
	}
	tl.padWidth = maxLen

	if tl.animated {
		text := fmt.Sprintf(`%s theme "%s"  [dry-run]`, tl.gerund(), tl.themeName)
		tl.writeLine(tl.colorDim(tl.glyphs.Top) + " " + tl.colorBright(text))
	} else {
		tl.writeLine(fmt.Sprintf(`%s theme "%s" [dry-run]`, tl.mode, tl.themeName))
	}

	totalUploads := 0
	for _, c := range categories {
		totalUploads += c.Count
		summary := fmt.Sprintf("%d would upload", c.Count)
		if tl.animated {
			tl.writeLine(tl.renderRail())
			marker := tl.colorDryRun(tl.glyphs.Pending)
			tl.writeLine(marker + " " + tl.formatCatLine(c.Label, summary))
		} else {
			tl.writeLine(fmt.Sprintf("  %s: %s", c.Label, summary))
		}
	}

	if deleteCount > 0 {
		summary := fmt.Sprintf("%d would delete", deleteCount)
		if tl.animated {
			tl.writeLine(tl.renderRail())
			marker := tl.colorDryRun(tl.glyphs.Pending)
			padded := fmt.Sprintf("%-*s", tl.padWidth, "deletes")
			tl.writeLine(marker + " " + tl.formatCatLine(padded, summary))
		} else {
			tl.writeLine(fmt.Sprintf("  deletes: %s", summary))
		}
	}

	footerText := fmt.Sprintf("%d %s, %d %s",
		totalUploads, plural(totalUploads, "upload", "uploads"),
		deleteCount, plural(deleteCount, "delete", "deletes"),
	)
	if tl.animated {
		tl.writeLine(tl.renderRail())
		tl.writeLine(tl.colorDim(tl.glyphs.Bottom) + " " + tl.colorDryRun("Dry run complete") + "  " + tl.colorDim(footerText))
		tl.writeLine("")
	} else {
		tl.writeLine(fmt.Sprintf("dry run complete: %s", footerText))
	}
	tl.closed = true
}

// NothingToDo renders a message when there are no files to process.
func (tl *SyncTimeline) NothingToDo() {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.started = time.Now()

	verb := "push"
	if tl.mode == "sync" {
		verb = "sync"
	}

	if tl.animated {
		text := fmt.Sprintf(`%s theme "%s"`, tl.gerund(), tl.themeName)
		tl.writeLine(tl.colorDim(tl.glyphs.Top) + " " + tl.colorBright(text))
		tl.writeLine(tl.renderRail())
		tl.writeLine(tl.colorDim(tl.glyphs.Bottom) + " " + tl.colorDim(fmt.Sprintf("Nothing to %s", verb)))
		tl.writeLine("")
	} else {
		tl.writeLine(fmt.Sprintf(`%s theme "%s"`, tl.mode, tl.themeName))
		tl.writeLine(fmt.Sprintf("nothing to %s", verb))
	}
	tl.closed = true
}

// Footer renders the closing success summary line. Idempotent.
func (tl *SyncTimeline) Footer() {
	tl.stopLoop()
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.closed {
		return
	}
	tl.closed = true
	tl.clearActiveLocked()

	elapsed := formatElapsedPrecise(time.Since(tl.started))
	total := tl.uploaded + tl.deleted

	var summary string
	if tl.skipped > 0 {
		summary = fmt.Sprintf("%d %s, %d skipped in %s", tl.uploaded, tl.pastVerb(), tl.skipped, elapsed)
		if tl.deleted > 0 {
			summary = fmt.Sprintf("%d %s, %d %s, %d skipped in %s",
				tl.uploaded, plural(tl.uploaded, "upload", "uploads"),
				tl.deleted, plural(tl.deleted, "delete", "deletes"),
				tl.skipped,
				elapsed,
			)
		}
	} else if tl.deleted > 0 {
		noun := "upload"
		nouns := "uploads"
		if tl.mode == "pull" {
			noun = "download"
			nouns = "downloads"
		}
		summary = fmt.Sprintf("%d %s, %d %s in %s",
			tl.uploaded, plural(tl.uploaded, noun, nouns),
			tl.deleted, plural(tl.deleted, "delete", "deletes"),
			elapsed,
		)
	} else {
		rate := ""
		if dur := time.Since(tl.started); dur >= 100*time.Millisecond && total > 0 {
			r := float64(total) / dur.Seconds()
			rate = fmt.Sprintf("  (%.1f/s)", r)
		}
		summary = fmt.Sprintf("%d %s in %s%s", total, plural(total, "file", "files"), elapsed, rate)
	}

	if tl.animated {
		tl.writeLine(tl.renderRail())
		tl.writeLine(tl.colorDim(tl.glyphs.Bottom) + " " + tl.colorDone("Done!") + "  " + tl.colorDim(summary))
		tl.writeLine("")
	} else {
		tl.writeLine(fmt.Sprintf("done: %s", summary))
	}
}

// ErrorFooter renders a closing error summary line. Idempotent.
func (tl *SyncTimeline) ErrorFooter() {
	tl.stopLoop()
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.closed {
		return
	}
	tl.closed = true
	tl.clearActiveLocked()

	summary := fmt.Sprintf("%d %s, %d failed, %d skipped", tl.uploaded, tl.pastVerb(), tl.failed, tl.skipped)

	if tl.animated {
		tl.writeLine(tl.renderRail())
		tl.writeLine(tl.colorDim(tl.glyphs.Bottom) + " " + tl.colorError("Error!") + "  " + tl.colorDim(summary))
		tl.writeLine("")
	} else {
		tl.writeLine(fmt.Sprintf("error: %s", summary))
	}
}

// Close stops the animation goroutine and clears the active display. Idempotent.
func (tl *SyncTimeline) Close() {
	tl.stopLoop()
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.clearActiveLocked()
}

// PreparePrompt clears live spinner output before a blocking prompt is printed.
func (tl *SyncTimeline) PreparePrompt() {
	tl.stopLoop()
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.clearActiveLocked()
}

// --- internal ---

func (tl *SyncTimeline) stopLoop() {
	tl.stopOnce.Do(func() {
		if tl.animated {
			close(tl.stopCh)
			<-tl.doneCh
		}
	})
}

func (tl *SyncTimeline) loop() {
	ticker := time.NewTicker(80 * time.Millisecond)
	defer func() {
		ticker.Stop()
		close(tl.doneCh)
	}()
	for {
		select {
		case <-ticker.C:
			tl.mu.Lock()
			if tl.activeCat >= 0 || tl.delState != nil {
				tl.renderActiveLocked()
			}
			tl.mu.Unlock()
		case <-tl.stopCh:
			return
		}
	}
}

func (tl *SyncTimeline) renderActiveLocked() {
	if !tl.animated || (tl.activeCat < 0 && tl.delState == nil) {
		return
	}
	if tl.lines == 0 && time.Since(tl.activeSince) < tl.spinnerDelay {
		return
	}

	tl.eraseActiveLinesLocked()

	spinner := tl.colorActive(tl.glyphs.Spinner[tl.frame%len(tl.glyphs.Spinner)])
	tl.frame++
	dash := tl.glyphs.Dash

	var head string
	if tl.delState != nil {
		label := fmt.Sprintf("%-*s", tl.padWidth, "Deleting")
		head = fmt.Sprintf("%s %s %s %d/%d", spinner, label, dash, tl.delState.done, tl.delState.total)
	} else if tl.activeCat >= 0 {
		cat := tl.cats[tl.activeCat]
		label := fmt.Sprintf("%-*s", tl.padWidth, cat.label)
		progress := cat.done + cat.skipped
		head = fmt.Sprintf("%s %s %s %d/%d", spinner, label, dash, progress, cat.total)
	}

	// Both live lines must fit on one row each: erasing moves up exactly
	// tl.lines rows, so a wrapped line would leave a stale row per redraw.
	width := liveWidth(tl.terminalWidth, tl.writer)
	line := fitLiveLine(head, width, tl.glyphs.Ellipsis)
	if tl.activeFile != "" {
		line = fitLiveTail(head, "  ", tl.activeFile, width, tl.glyphs.Ellipsis)
	}

	_, _ = fmt.Fprint(tl.writer, fitLiveLine(tl.renderRail(), width, tl.glyphs.Ellipsis)+"\n")
	_, _ = fmt.Fprint(tl.writer, line+"\n")
	tl.lines = 2
}

func (tl *SyncTimeline) clearActiveLocked() {
	if !tl.animated || tl.lines == 0 {
		return
	}
	tl.eraseActiveLinesLocked()
	tl.lines = 0
}

func (tl *SyncTimeline) eraseActiveLinesLocked() {
	for i := 0; i < tl.lines; i++ {
		_, _ = fmt.Fprint(tl.writer, "\033[1A\033[2K")
	}
	_, _ = fmt.Fprint(tl.writer, "\r")
}

func (tl *SyncTimeline) formatCatLine(label, summary string) string {
	padded := fmt.Sprintf("%-*s", tl.padWidth, label)
	return tl.colorBright(padded) + " " + tl.colorDim(tl.glyphs.Dash) + " " + summary
}

func (tl *SyncTimeline) renderRail() string {
	return tl.colorDim(tl.glyphs.Rail)
}

func (tl *SyncTimeline) writeLine(line string) {
	_, _ = fmt.Fprintln(tl.writer, line)
}

// --- verb helpers ---

func (tl *SyncTimeline) pastVerb() string {
	if tl.mode == "pull" {
		return "downloaded"
	}
	return "uploaded"
}

func (tl *SyncTimeline) gerund() string {
	switch tl.mode {
	case "pull":
		return "Pulling"
	case "sync":
		return "Syncing"
	default:
		return "Pushing"
	}
}

// --- color helpers ---

func (tl *SyncTimeline) colorDone(s string) string {
	if tl.dryRun {
		return tl.theme.Warning(s)
	}
	return tl.theme.Success(s)
}

func (tl *SyncTimeline) colorActive(s string) string {
	if tl.dryRun {
		return tl.theme.Warning(s)
	}
	return tl.theme.Accent(s)
}

func (tl *SyncTimeline) colorError(s string) string { return tl.theme.Error(s) }

func (tl *SyncTimeline) colorDryRun(s string) string { return tl.theme.Warning(s) }

func (tl *SyncTimeline) colorDim(s string) string { return tl.theme.Muted(s) }

func (tl *SyncTimeline) colorBright(s string) string { return tl.theme.Strong(s) }

// --- helpers ---

func formatElapsedPrecise(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < 10*time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return formatElapsed(d)
}

func plural(n int, singular, p string) string {
	if n == 1 {
		return singular
	}
	return p
}
