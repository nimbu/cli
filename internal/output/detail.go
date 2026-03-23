package output

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"text/tabwriter"
)

// Field represents one row in a detail view.
type Field struct {
	Label  string
	Value  any
	always bool
}

// F creates a Field that is skipped when the value is the zero value for its type.
func F(label string, value any) Field {
	return Field{Label: label, Value: value}
}

// FAlways creates a Field that is always displayed, even when zero.
func FAlways(label string, value any) Field {
	return Field{Label: label, Value: value, always: true}
}

// Detail writes a single resource across all three output modes.
// JSON mode writes jsonData, Plain mode writes plainValues as TSV,
// and Human mode renders fields as aligned key-value pairs.
func Detail(ctx context.Context, jsonData any, plainValues []any, fields []Field) error {
	mode := FromContext(ctx)

	switch {
	case mode.JSON:
		return JSON(ctx, jsonData)
	case mode.Plain:
		return Plain(ctx, plainValues...)
	default:
		w := WriterFromContext(ctx)
		return WriteDetail(w.Out, fields)
	}
}

// WriteDetail writes fields as aligned "Label:  Value" pairs.
// Fields created with F() are skipped when their value is the zero value.
func WriteDetail(w io.Writer, fields []Field) error {
	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)

	for _, f := range fields {
		if !f.always && isZero(f.Value) {
			continue
		}
		if _, err := fmt.Fprintf(tw, "%s:\t%v\n", f.Label, f.Value); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func isZero(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return rv.IsZero()
	}
}
