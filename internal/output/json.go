package output

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// WriteJSON writes v as indented JSON to the writer.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}

	return nil
}

// JSON writes v as JSON to the context's writer.
func JSON(ctx context.Context, v any) error {
	w := WriterFromContext(ctx)
	return WriteJSON(w.Out, v)
}

// JSONErr writes v as JSON to stderr.
func JSONErr(ctx context.Context, v any) error {
	w := WriterFromContext(ctx)
	return WriteJSON(w.Err, v)
}

// StatusPayload creates a simple status object for JSON output.
func StatusPayload(status string, message string) map[string]any {
	m := map[string]any{"status": status}
	if message != "" {
		m["message"] = message
	}
	return m
}

// SuccessPayload creates a success status object.
func SuccessPayload(message string) map[string]any {
	return StatusPayload("success", message)
}

// ErrorPayload creates an error status object.
func ErrorPayload(err error) map[string]any {
	return map[string]any{
		"status": "error",
		"error":  err.Error(),
	}
}

// CountPayload creates a count result object.
func CountPayload(count int) map[string]any {
	return map[string]any{"count": count}
}

// IDPayload creates an ID result object.
func IDPayload(id string) map[string]any {
	return map[string]any{"id": id}
}

// PathPayload creates a path result object.
func PathPayload(path string) map[string]any {
	return map[string]any{"path": path}
}
