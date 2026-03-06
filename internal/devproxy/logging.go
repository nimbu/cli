package devproxy

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Logger provides human and structured runtime output.
type Logger struct {
	DebugEnabled bool
	EventsJSON   bool
	UseColor     bool
	mu           sync.Mutex
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
			_, _ = fmt.Fprintln(os.Stderr, string(data))
			return
		}
	}

	if len(fields) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, message)
		return
	}

	_, _ = fmt.Fprintf(os.Stderr, "%s %v\n", message, fields)
}

func requestLogLine(method string, path string, status int, useColor bool) string {
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if !useColor {
		return fmt.Sprintf("%s %s %s (%d)", ts, method, path, status)
	}

	statusText := fmt.Sprintf("(%d)", status)
	switch {
	case status >= 200 && status < 400:
		statusText = ansiGreen(statusText)
	case status >= 400 && status < 500:
		statusText = ansiYellow(statusText)
	default:
		statusText = ansiRed(statusText)
	}

	return fmt.Sprintf("%s %s %s %s", ansiDim(ts), method, path, statusText)
}

func ansiDim(value string) string {
	return ansiWrap("2", value)
}

func ansiGreen(value string) string {
	return ansiWrap("32", value)
}

func ansiYellow(value string) string {
	return ansiWrap("33", value)
}

func ansiRed(value string) string {
	return ansiWrap("31", value)
}

func ansiWrap(code string, value string) string {
	if value == "" {
		return value
	}
	return fmt.Sprintf("\x1b[%sm%s\x1b[0m", code, value)
}
