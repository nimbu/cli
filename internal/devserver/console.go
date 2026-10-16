package devserver

import (
	"bytes"
	"io"
	"sync"
	"time"
)

const (
	// consolePartialFlushDelay is how long an unterminated line (a prompt, a
	// progress counter) waits for its newline before it is shown anyway.
	consolePartialFlushDelay = 100 * time.Millisecond
	// consoleMaxPending caps how much of one unterminated line is buffered.
	consoleMaxPending = 32 << 10
)

// Console serializes line-oriented output from several sources onto shared
// terminal streams: the child dev server, the proxy request log and nimbu's
// own notices. Every source is line-buffered and writes whole lines under one
// lock, so no line ever lands in the middle of another.
//
// A source whose line stays unterminated (a "Compiling..." prompt) is shown
// after a short delay. When another source then needs the screen, the open
// line is ended first; when the first source continues, its prefix is
// repeated on the new line.
type Console struct {
	mu         sync.Mutex
	open       *ConsoleWriter
	flushDelay time.Duration
}

// NewConsole returns a Console with the default partial-line delay.
func NewConsole() *Console {
	return &Console{flushDelay: consolePartialFlushDelay}
}

// ConsoleWriter is one output source attached to a Console.
type ConsoleWriter struct {
	console *Console
	dst     io.Writer
	prefix  []byte
	pending []byte
	timer   *time.Timer
	err     error
}

// Writer returns a source that writes to dst, starting every line with
// prefix. prefix may carry ANSI styling; an empty prefix passes lines through
// unchanged.
func (c *Console) Writer(dst io.Writer, prefix string) *ConsoleWriter {
	return &ConsoleWriter{console: c, dst: dst, prefix: []byte(prefix)}
}

// Write buffers p and emits every complete line. It never returns a short
// count: output errors are sticky and reported on the next Write.
func (w *ConsoleWriter) Write(p []byte) (int, error) {
	c := w.console
	c.mu.Lock()
	defer c.mu.Unlock()

	n := len(p)
	for {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			break
		}
		w.pending = append(w.pending, p[:i]...)
		p = p[i+1:]
		c.emitLocked(w, w.pending, true)
		w.pending = w.pending[:0]
	}
	w.pending = append(w.pending, p...)
	w.stopTimerLocked()
	if len(w.pending) >= consoleMaxPending {
		c.emitLocked(w, w.pending, false)
		w.pending = w.pending[:0]
	}
	if len(w.pending) > 0 {
		w.timer = time.AfterFunc(c.flushDelay, w.flushPartial)
	}
	return n, w.err
}

// Close emits any buffered partial line and ends a line left open on screen.
func (w *ConsoleWriter) Close() error {
	c := w.console
	c.mu.Lock()
	defer c.mu.Unlock()
	w.stopTimerLocked()
	switch {
	case len(w.pending) > 0:
		c.emitLocked(w, w.pending, true)
		w.pending = w.pending[:0]
	case c.open == w:
		w.writeLocked([]byte{'\n'})
		c.open = nil
	}
	return w.err
}

func (w *ConsoleWriter) flushPartial() {
	c := w.console
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(w.pending) == 0 {
		return
	}
	c.emitLocked(w, w.pending, false)
	w.pending = w.pending[:0]
}

func (w *ConsoleWriter) stopTimerLocked() {
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
}

func (w *ConsoleWriter) writeLocked(b []byte) {
	if _, err := w.dst.Write(b); err != nil && w.err == nil {
		w.err = err
	}
}

// emitLocked writes data for w as one Write call. terminated reports whether
// data is a complete line (its newline was stripped).
func (c *Console) emitLocked(w *ConsoleWriter, data []byte, terminated bool) {
	if c.open != nil && c.open != w {
		c.open.writeLocked([]byte{'\n'})
		c.open = nil
	}

	if terminated && len(data) > 0 && data[len(data)-1] == '\r' {
		data = data[:len(data)-1] // CRLF
	}

	out := make([]byte, 0, len(w.prefix)+len(data)+1)
	if c.open != w {
		out = append(out, w.prefix...)
	}
	if len(w.prefix) > 0 && bytes.IndexByte(data, '\r') >= 0 {
		// A bare CR redraws the line from column 0 (spinners, progress
		// counters); repeat the prefix so the redraw does not erase it.
		withPrefix := make([]byte, 0, len(w.prefix)+1)
		withPrefix = append(withPrefix, '\r')
		withPrefix = append(withPrefix, w.prefix...)
		data = bytes.ReplaceAll(data, []byte{'\r'}, withPrefix)
	}
	out = append(out, data...)

	switch {
	case terminated:
		out = append(out, '\n')
		c.open = nil
	case len(out) > 0:
		c.open = w
	}
	if len(out) > 0 {
		w.writeLocked(out)
	}
}
