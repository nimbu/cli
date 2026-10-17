package output

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
)

type progressCtxKey struct{}

// clearLine returns the cursor to column 0 and erases the line.
const clearLine = "\r\033[2K"

// Progress renders lightweight human-only progress feedback on stderr.
//
// On an animated terminal the active task is redrawn in place on one line
// every tick. The line only appears once a task has run for spinnerDelay, so
// near-instant work never flashes a spinner.
//
// Progress installs no signal handler of its own: the command layer owns
// SIGINT/SIGTERM and cancels the context on the first one. When that context
// is canceled the live line is erased and drawing stops, so Ctrl-C never
// leaves a half-drawn line under the shell prompt or under the error that
// follows.
type Progress struct {
	mu           sync.Mutex
	writer       io.Writer
	theme        Theme
	glyphs       Glyphs
	enabled      bool
	animated     bool
	spinnerDelay time.Duration
	started      time.Time
	frame        int
	task         *Task
	lineOpen     bool // an animated line is drawn without its trailing newline
	// terminalWidth overrides the detected width the live line is fitted to
	// (tests); 0 detects it from the writer.
	terminalWidth int
	interrupted   bool
	cancel        <-chan struct{} // closed when the command is interrupted
	stopCh        chan struct{}
	doneCh        chan struct{}
	closeOnce     sync.Once
}

// Task represents one in-flight spinner, counter, or transfer.
type Task struct {
	progress *Progress
	parent   *Task
	kind     taskKind
	label    string
	total    int64
	current  int64
	started  time.Time
	done     bool
	status   string
}

type taskKind uint8

const (
	taskSpinner taskKind = iota
	taskCounter
	taskTransfer
)

// NewProgress builds a human-mode stderr progress session for the current context.
func NewProgress(ctx context.Context) *Progress {
	writer := WriterFromContext(ctx)
	if !IsHuman(ctx) || writer == nil || writer.NoSpin {
		return &Progress{}
	}

	out := writer.Err
	if out == nil {
		out = writer.Out
	}
	return newProgress(ctx.Done(), out, writer.ErrTheme(), writer.Glyphs(), writer.ErrAnimated())
}

// newProgress builds a progress session; cancel (may be nil) ends the live
// line early when the command is interrupted.
func newProgress(cancel <-chan struct{}, out io.Writer, theme Theme, glyphs Glyphs, animated bool) *Progress {
	p := &Progress{
		writer:       out,
		theme:        theme,
		glyphs:       glyphs,
		enabled:      true,
		animated:     animated,
		spinnerDelay: spinnerDelay,
		cancel:       cancel,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
	if p.animated {
		go p.loop()
	}
	return p
}

// NewDisabledProgress returns a no-op progress that silently discards all output.
// Use when a timeline renderer handles progress display instead.
func NewDisabledProgress() *Progress {
	return &Progress{}
}

// WithProgress stores a progress session in context.
func WithProgress(ctx context.Context, p *Progress) context.Context {
	return context.WithValue(ctx, progressCtxKey{}, p)
}

// ProgressFromContext extracts the progress session from context.
func ProgressFromContext(ctx context.Context) *Progress {
	if v := ctx.Value(progressCtxKey{}); v != nil {
		if p, ok := v.(*Progress); ok {
			return p
		}
	}
	return &Progress{}
}

// Close stops background rendering and leaves stderr clean: a task that was
// never finished has its live line erased rather than reported as done.
// Close is idempotent.
func (p *Progress) Close() {
	if p == nil || !p.animated {
		return
	}
	p.closeOnce.Do(func() {
		close(p.stopCh)
		<-p.doneCh
	})
}

// Phase starts a spinner task.
func (p *Progress) Phase(label string) *Task {
	return p.start(taskSpinner, label, 0)
}

// Counter starts an item counter task.
func (p *Progress) Counter(label string, total int64) *Task {
	return p.start(taskCounter, label, total)
}

// Transfer starts a byte-progress task.
func (p *Progress) Transfer(label string, total int64) *Task {
	return p.start(taskTransfer, label, total)
}

func (p *Progress) start(kind taskKind, label string, total int64) *Task {
	task := &Task{
		progress: p,
		kind:     kind,
		label:    strings.TrimSpace(label),
		total:    total,
		started:  time.Now(),
	}
	if p == nil || !p.enabled {
		return task
	}

	p.mu.Lock()
	task.parent = p.task
	p.task = task
	p.started = task.started
	if !p.animated {
		_, _ = fmt.Fprintf(p.writer, "%s...\n", task.label)
	}
	p.mu.Unlock()
	return task
}

func (p *Progress) loop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer func() {
		ticker.Stop()
		close(p.doneCh)
	}()

	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			p.renderLocked(false)
			p.mu.Unlock()
		case <-p.cancel:
			// Interrupted: erase the live line and stop drawing, so the
			// command's error or summary starts on a clean line.
			p.mu.Lock()
			p.interrupted = true
			p.eraseLineLocked()
			p.mu.Unlock()
			return
		case <-p.stopCh:
			p.mu.Lock()
			p.eraseLineLocked()
			p.mu.Unlock()
			return
		}
	}
}

// eraseLineLocked clears an open animated line.
func (p *Progress) eraseLineLocked() {
	if p.lineOpen {
		_, _ = fmt.Fprint(p.writer, clearLine)
	}
	p.lineOpen = false
}

func (p *Progress) renderLocked(final bool) {
	if !p.enabled || p.interrupted || p.task == nil {
		return
	}
	finished := final || p.task.done

	if p.animated {
		if !finished && !p.lineOpen && time.Since(p.task.started) < p.spinnerDelay {
			return
		}
		// clearLine only clears the cursor's row, so the live line must
		// never wrap. The final line is permanent and may wrap.
		var line string
		if finished {
			line = p.renderLineLocked(final)
		} else {
			line = p.fitLiveLineLocked(final)
		}
		_, _ = fmt.Fprint(p.writer, clearLine+line)
		if finished {
			_, _ = fmt.Fprint(p.writer, "\n")
		}
		p.lineOpen = !finished
		return
	}

	if finished {
		_, _ = fmt.Fprintln(p.writer, p.renderLineLocked(final))
	}
}

func (p *Progress) renderLineLocked(final bool) string {
	prefix, label, metrics := p.renderPartsLocked(final)
	return strings.Join(filterEmpty(append([]string{prefix, label}, metrics...)), "  ")
}

// fitLiveLineLocked renders the live line fitted to the terminal width,
// shortening the label (often a file path) from the left first.
func (p *Progress) fitLiveLineLocked(final bool) string {
	prefix, label, metrics := p.renderPartsLocked(final)
	width := liveWidth(p.terminalWidth, p.writer)
	tail := strings.Join(filterEmpty(metrics), "  ")
	if label == "" || width <= 1 {
		return strings.Join(filterEmpty([]string{prefix, label, tail}), "  ")
	}
	// Fit "prefix  label" so the label's end survives, then append metrics.
	reserve := 0
	if tail != "" {
		reserve = ansi.StringWidth(tail) + 2
	}
	head := fitLiveTail(prefix, "  ", label, width-reserve, p.glyphs.Ellipsis)
	return fitLiveLine(strings.Join(filterEmpty([]string{head, tail}), "  "), width, p.glyphs.Ellipsis)
}

func (p *Progress) renderPartsLocked(final bool) (prefix, label string, metrics []string) {
	task := p.task
	if task == nil {
		return "", "", nil
	}
	elapsed := time.Since(task.started)

	if final || task.done {
		if task.status == "" {
			task.status = "done"
		}
		if task.status == "failed" {
			prefix = p.theme.Error(p.glyphs.Fail + " " + task.status)
		} else {
			prefix = p.theme.Success(p.glyphs.OK + " " + task.status)
		}
	} else {
		prefix = p.theme.Accent(p.spinnerFrameLocked())
	}

	switch task.kind {
	case taskCounter:
		metrics = renderCounterMetrics(task.current, task.total, elapsed)
	case taskTransfer:
		metrics = renderTransferMetrics(task.current, task.total, elapsed)
	default:
		metrics = []string{formatElapsed(elapsed)}
	}
	return prefix, task.label, metrics
}

func (p *Progress) spinnerFrameLocked() string {
	frames := p.glyphs.Spinner
	if len(frames) == 0 {
		frames = asciiGlyphs.Spinner
	}
	frame := frames[p.frame%len(frames)]
	p.frame++
	return frame
}

// SetLabel updates the task label.
func (t *Task) SetLabel(label string) {
	if t == nil {
		return
	}
	t.withProgress(func(p *Progress) {
		t.label = strings.TrimSpace(label)
	})
}

// SetTotal updates the known total.
func (t *Task) SetTotal(total int64) {
	if t == nil {
		return
	}
	t.withProgress(func(p *Progress) {
		t.total = total
	})
}

// Add advances the task by n units/bytes.
func (t *Task) Add(n int64) {
	if t == nil || n <= 0 {
		return
	}
	t.withProgress(func(p *Progress) {
		t.current += n
	})
}

// Current returns the current progress amount.
func (t *Task) Current() int64 {
	if t == nil {
		return 0
	}
	var current int64
	t.withProgress(func(_ *Progress) {
		current = t.current
	})
	return current
}

// ResetProgress resets current progress and elapsed timing for a retry/replay attempt.
func (t *Task) ResetProgress() {
	if t == nil {
		return
	}
	p := t.progress
	if p == nil || !p.enabled {
		t.current = 0
		t.started = time.Now()
		return
	}
	p.mu.Lock()
	t.current = 0
	t.started = time.Now()
	p.mu.Unlock()
}

// Done finalizes the task with an optional status label.
func (t *Task) Done(status string) {
	if t == nil {
		return
	}
	if strings.TrimSpace(status) == "" {
		status = "done"
	}
	t.finish(status)
}

// Fail finalizes the task as failed.
func (t *Task) Fail(err error) {
	if t == nil {
		return
	}
	status := "failed"
	if err != nil && err.Error() != "" {
		status = "failed"
	}
	t.finish(status)
}

// WrapReader increments the task as data is read.
func (t *Task) WrapReader(reader io.Reader) io.Reader {
	if t == nil || reader == nil {
		return reader
	}
	return &progressReader{reader: reader, task: t}
}

// WrapReadCloser increments the task as data is read and preserves Close.
func (t *Task) WrapReadCloser(reader io.ReadCloser) io.ReadCloser {
	if t == nil || reader == nil {
		return reader
	}
	return &progressReadCloser{progressReader: progressReader{reader: reader, task: t}, closer: reader}
}

// WrapWriter increments the task as data is written.
func (t *Task) WrapWriter(writer io.Writer) io.Writer {
	if t == nil || writer == nil {
		return writer
	}
	return &progressWriter{writer: writer, task: t}
}

func (t *Task) finish(status string) {
	p := t.progress
	if p == nil || !p.enabled {
		if t.done {
			return
		}
		t.done = true
		t.status = status
		return
	}

	p.mu.Lock()
	if t.done {
		p.mu.Unlock()
		return
	}
	t.done = true
	t.status = status
	if p.task == t {
		p.renderLocked(true)
		p.task = t.parent
		if p.task != nil && !p.animated {
			_, _ = fmt.Fprintf(p.writer, "%s...\n", p.task.label)
		}
	} else {
		t.parent = nil
	}
	p.mu.Unlock()
}

func (t *Task) withProgress(fn func(*Progress)) {
	p := t.progress
	if p == nil || !p.enabled {
		fn(&Progress{})
		return
	}
	p.mu.Lock()
	fn(p)
	p.mu.Unlock()
}

type progressReader struct {
	reader io.Reader
	task   *Task
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.task.Add(int64(n))
	}
	return n, err
}

type progressReadCloser struct {
	progressReader
	closer io.Closer
}

func (r *progressReadCloser) Close() error {
	return r.closer.Close()
}

type progressWriter struct {
	writer io.Writer
	task   *Task
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 {
		w.task.Add(int64(n))
	}
	return n, err
}

func renderCounterMetrics(current, total int64, elapsed time.Duration) []string {
	parts := []string{fmt.Sprintf("%d", current)}
	if total > 0 {
		parts[0] = fmt.Sprintf("%d/%d", current, total)
	}
	if elapsed > 0 && current > 0 {
		rate := float64(current) / elapsed.Seconds()
		parts = append(parts, fmt.Sprintf("%.1f/s", rate))
		if total > 0 && current < total && rate > 0 {
			remaining := time.Duration(float64(total-current)/rate) * time.Second
			parts = append(parts, "eta "+formatElapsed(remaining))
		}
	}
	parts = append(parts, formatElapsed(elapsed))
	return parts
}

func renderTransferMetrics(current, total int64, elapsed time.Duration) []string {
	parts := []string{formatBytes(current)}
	if total > 0 {
		parts[0] = fmt.Sprintf("%s/%s", formatBytes(current), formatBytes(total))
	}
	if elapsed > 0 && current > 0 {
		rate := float64(current) / elapsed.Seconds()
		parts = append(parts, fmt.Sprintf("%s/s", formatBytes(int64(rate))))
		if total > 0 && current < total && rate > 0 {
			remaining := time.Duration(float64(total-current)/rate) * time.Second
			parts = append(parts, "eta "+formatElapsed(remaining))
		}
	}
	parts = append(parts, formatElapsed(elapsed))
	return parts
}

func filterEmpty(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			items = append(items, value)
		}
	}
	return items
}

func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	minutes := int(d / time.Minute)
	seconds := int((d % time.Minute) / time.Second)
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	}
	hours := int(d / time.Hour)
	minutes = int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%dh%02dm", hours, minutes)
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(n)
	for _, unit := range units {
		value /= 1024
		if value < 1024 || unit == units[len(units)-1] {
			if value >= 100 {
				return fmt.Sprintf("%.0f %s", math.Round(value), unit)
			}
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%d B", n)
}
