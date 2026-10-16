package output

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/charmbracelet/x/ansi"
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
// Labels are padded by display width, and continuation lines of multi-line
// values are indented under the value column.
func WriteDetail(w io.Writer, fields []Field) error {
	visible := make([]Field, 0, len(fields))
	labelWidth := 0
	for _, f := range fields {
		if !f.always && isZero(f.Value) {
			continue
		}
		visible = append(visible, f)
		if lw := ansi.StringWidth(f.Label) + 1; lw > labelWidth {
			labelWidth = lw
		}
	}

	indent := strings.Repeat(" ", labelWidth+1)
	for _, f := range visible {
		label := f.Label + ":"
		pad := strings.Repeat(" ", labelWidth-ansi.StringWidth(label)+1)
		value := detailNewlines.Replace(fmt.Sprint(f.Value))
		value = strings.ReplaceAll(value, "\n", "\n"+indent)
		if _, err := fmt.Fprintln(w, strings.TrimRight(label+pad+value, " ")); err != nil {
			return err
		}
	}
	return nil
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

// detailNewlines normalizes CRLF and lone CR to LF, so a carriage return in a
// value cannot jump back to column 0 and overwrite its label.
var detailNewlines = strings.NewReplacer("\r\n", "\n", "\r", "\n")
