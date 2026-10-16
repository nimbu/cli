package devproxy

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/muesli/termenv"
)

// Logger provides human and structured runtime output.
type Logger struct {
	DebugEnabled bool
	EventsJSON   bool
	UseColor     bool
	// Out receives diagnostics; nil means os.Stderr.
	Out io.Writer
	mu  sync.Mutex
}

func (l *Logger) out() io.Writer {
	if l.Out != nil {
		return l.Out
	}
	return os.Stderr
}

func (l *Logger) Debug(message string, fields map[string]any) {
	if !l.DebugEnabled {
		return
	}
	l.emit("debug", message, fields)
}

func (l *Logger) Info(message string, fields map[string]any) {
	l.emit("info", message, fields)
}

func (l *Logger) Warn(message string, fields map[string]any) {
	l.emit("warn", message, fields)
}

func (l *Logger) emit(level string, message string, fields map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.EventsJSON {
		payload := map[string]any{
			"ts":      time.Now().UTC().Format(time.RFC3339Nano),
			"level":   level,
			"message": message,
		}
		for k, v := range fields {
			payload[k] = v
		}
		data, err := json.Marshal(payload)
		if err == nil {
			_, _ = fmt.Fprintln(l.out(), string(data))
			return
		}
	}

	if len(fields) == 0 {
		_, _ = fmt.Fprintln(l.out(), message)
		return
	}

	_, _ = fmt.Fprintf(l.out(), "%s %v\n", message, fields)
}

// requestLogLine formats one proxy request log line. Color is decided by the
// caller for the stream the line goes to (stdout); the status colors are the
// basic 16, so the ANSI profile renders them on every color terminal.
func requestLogLine(method string, path string, status int, useColor bool) string {
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if !useColor {
		return fmt.Sprintf("%s %s %s (%d)", ts, method, path, status)
	}

	profile := termenv.ANSI
	statusStyle := termenv.String(fmt.Sprintf("(%d)", status))
	switch {
	case status >= 200 && status < 400:
		statusStyle = statusStyle.Foreground(profile.Color("2"))
	case status >= 400 && status < 500:
		statusStyle = statusStyle.Foreground(profile.Color("3"))
	default:
		statusStyle = statusStyle.Foreground(profile.Color("1"))
	}

	return fmt.Sprintf("%s %s %s %s", termenv.String(ts).Faint(), method, path, statusStyle)
}
