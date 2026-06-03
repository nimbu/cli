package output

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
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

// JSONFromSliceFields writes a slice as JSON with only the requested fields.
func JSONFromSliceFields(ctx context.Context, slice any, fields []string) error {
	projected, err := projectSliceFields(slice, fields)
	if err != nil {
		return err
	}
	return JSON(ctx, projected)
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

func projectSliceFields(slice any, fields []string) ([]map[string]any, error) {
	rv := reflect.ValueOf(slice)
	if rv.Kind() != reflect.Slice {
		return nil, fmt.Errorf("expected slice, got %T", slice)
	}

	if err := validateFieldsForSliceType(rv.Type(), fields); err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		row, err := projectStructFields(rv.Index(i), fields)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

func projectStructFields(rv reflect.Value, fields []string) (map[string]any, error) {
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return map[string]any{}, nil
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return map[string]any{"value": fmt.Sprint(rv.Interface())}, nil
	}

	if err := validateFieldNames(rv.Type(), fields); err != nil {
		return nil, err
	}

	rt := rv.Type()
	out := make(map[string]any, len(fields))
	for _, fieldName := range fields {
		for i := 0; i < rt.NumField(); i++ {
			sf := rt.Field(i)
			jsonName := strings.Split(sf.Tag.Get("json"), ",")[0]
			if jsonName == fieldName || strings.EqualFold(sf.Name, fieldName) {
				key := jsonName
				if key == "" {
					key = fieldName
				}
				out[key] = rv.Field(i).Interface()
				break
			}
		}
	}
	return out, nil
}
