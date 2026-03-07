package output

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/muesli/termenv"
)

type progressCtxKey struct{}

var spinnerFrames = []string{"-", "\\", "|", "/"}

// Progress renders lightweight human-only progress feedback on stderr.
type Progress struct {
	mu       sync.Mutex
	writer   io.Writer
	termOut  *termenv.Output
	enabled  bool
	animated bool
	useColor bool
	started  time.Time
	frame    int
	task     *Task
	stopCh   chan struct{}
	doneCh   chan struct{}
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

	profile := termenv.Ascii
	useColor := writer.Color == "always" || (writer.Color != "never" && writer.ErrIsTTY())
	if useColor {
		switch writer.Color {
		case "always":
			profile = termenv.TrueColor
		default:
			profile = termenv.EnvColorProfile()
		}
	}

	p := &Progress{
		writer:   out,
		termOut:  termenv.NewOutput(out, termenv.WithProfile(profile)),
		enabled:  true,
		animated: writer.ErrIsTTY(),
		useColor: useColor,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
	if p.animated {
		go p.loop()
	}
	return p
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

// Close stops background rendering and leaves stderr clean.
func (p *Progress) Close() {
	if p == nil || !p.animated {
		return
	}
	close(p.stopCh)
	<-p.doneCh
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
		case <-p.stopCh:
			p.mu.Lock()
			if p.task != nil && !p.task.done {
				p.task.done = true
				if p.task.status == "" {
					p.task.status = "done"
				}
			}
			p.renderLocked(true)
			p.mu.Unlock()
			return
		}
	}
}

func (p *Progress) renderLocked(final bool) {
	if !p.enabled || p.task == nil {
		return
	}

	line := p.renderLineLocked(final)
	if p.animated {
		_, _ = fmt.Fprintf(p.writer, "\r\033[2K%s", line)
		if final || p.task.done {
			_, _ = fmt.Fprint(p.writer, "\n")
		}
		return
	}

	if final || p.task.done {
		_, _ = fmt.Fprintln(p.writer, line)
	}
}

func (p *Progress) renderLineLocked(final bool) string {
	task := p.task
	if task == nil {
		return ""
	}
	elapsed := time.Since(task.started)

	prefix := p.spinnerFrameLocked()
	if final || task.done {
		if task.status == "" {
			task.status = "done"
		}
		prefix = task.status
	}
	if p.useColor {
		prefix = p.termOut.String(prefix).Foreground(p.termOut.Color("#38bdf8")).Bold().String()
	}

	parts := []string{prefix, task.label}
	switch task.kind {
	case taskCounter:
		parts = append(parts, renderCounterMetrics(task.current, task.total, elapsed)...)
	case taskTransfer:
		parts = append(parts, renderTransferMetrics(task.current, task.total, elapsed)...)
	default:
		parts = append(parts, formatElapsed(elapsed))
	}
	return strings.Join(filterEmpty(parts), "  ")
}

func (p *Progress) spinnerFrameLocked() string {
	frame := spinnerFrames[p.frame%len(spinnerFrames)]
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
		p.renderLocked(false)
	})
}

// SetTotal updates the known total.
func (t *Task) SetTotal(total int64) {
	if t == nil {
		return
	}
	t.withProgress(func(p *Progress) {
		t.total = total
		p.renderLocked(false)
	})
}

// Add advances the task by n units/bytes.
func (t *Task) Add(n int64) {
	if t == nil || n <= 0 {
		return
	}
	t.withProgress(func(p *Progress) {
		t.current += n
		p.renderLocked(false)
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
	p.renderLocked(false)
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
