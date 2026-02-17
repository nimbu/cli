package output

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/tabwriter"
)

// Table represents a simple table for human-readable output.
type Table struct {
	w       *tabwriter.Writer
	headers []string
}

// NewTable creates a new table writer.
func NewTable(out io.Writer, headers ...string) *Table {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	t := &Table{
		w:       tw,
		headers: headers,
	}

	if len(headers) > 0 {
		t.writeRow(headers)
	}

	return t
}

// Row adds a row to the table.
func (t *Table) Row(values ...any) {
	strs := make([]string, len(values))
	for i, v := range values {
		strs[i] = formatValue(v)
	}
	t.writeRow(strs)
}

func (t *Table) writeRow(values []string) {
	_, _ = fmt.Fprintln(t.w, strings.Join(values, "\t"))
}

// Flush writes the table to the underlying writer.
func (t *Table) Flush() error {
	return t.w.Flush()
}

// WriteTable writes a slice of structs as a table.
func WriteTable(ctx context.Context, slice any, fields []string, headers []string) error {
	w := WriterFromContext(ctx)

	rv := reflect.ValueOf(slice)
	if rv.Kind() != reflect.Slice {
		return fmt.Errorf("expected slice, got %T", slice)
	}

	if err := validateFieldsForSliceType(rv.Type(), fields); err != nil {
		return err
	}

	if len(headers) == 0 {
		headers = fields
	}

	t := NewTable(w.Out, headers...)

	for i := 0; i < rv.Len(); i++ {
		values, err := extractFields(rv.Index(i).Interface(), fields)
		if err != nil {
			return err
		}
		t.Row(values...)
	}

	return t.Flush()
}

// formatValue formats a value for table display.
func formatValue(v any) string {
	if v == nil {
		return ""
	}

	rv := reflect.ValueOf(v)

	// Handle pointers
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
		v = rv.Interface()
	}

	switch val := v.(type) {
	case bool:
		if val {
			return "yes"
		}
		return "no"
	case []string:
		return strings.Join(val, ", ")
	default:
		return fmt.Sprint(val)
	}
}

// Print writes to stdout based on output mode.
func Print(ctx context.Context, jsonData any, plainValues []any, humanFn func() error) error {
	mode := FromContext(ctx)

	switch {
	case mode.JSON:
		return JSON(ctx, jsonData)
	case mode.Plain:
		return Plain(ctx, plainValues...)
	default:
		return humanFn()
	}
}

// PrintSlice writes a slice based on output mode.
func PrintSlice(ctx context.Context, slice any, fields []string, headers []string) error {
	mode := FromContext(ctx)

	switch {
	case mode.JSON:
		return JSON(ctx, slice)
	case mode.Plain:
		return PlainFromSlice(ctx, slice, fields)
	default:
		return WriteTable(ctx, slice, fields, headers)
	}
}
