package output

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// columnGap is the number of spaces between table columns.
const columnGap = 2

// minTruncatedWidth is the narrowest a column is squeezed to when fitting a
// table to the terminal.
const minTruncatedWidth = 6

// Table renders aligned, borderless columns for human output.
//
// Widths are measured in terminal cells (East Asian wide characters and emoji
// count as two, ANSI escapes as zero), so mixed-script rows stay aligned. With
// a max width set (stdout is a terminal), rows wider than the terminal are
// fitted by truncating the widest non-ID columns with an ellipsis; piped
// output is never truncated.
type Table struct {
	out      io.Writer
	headers  []string
	rows     [][]string
	maxWidth int
	ellipsis string
}

// NewTable creates a new table writer. Rows are buffered until Flush.
func NewTable(out io.Writer, headers ...string) *Table {
	t := &Table{out: out, ellipsis: "…"}
	if len(headers) > 0 {
		t.headers = sanitizeCells(headers)
	}
	return t
}

// SetMaxWidth fits rows to width terminal cells; 0 disables truncation.
func (t *Table) SetMaxWidth(width int) { t.maxWidth = width }

// SetEllipsis sets the truncation marker.
func (t *Table) SetEllipsis(ellipsis string) { t.ellipsis = ellipsis }

// Row adds a row to the table.
func (t *Table) Row(values ...any) {
	strs := make([]string, len(values))
	for i, v := range values {
		strs[i] = formatValue(v)
	}
	t.rows = append(t.rows, sanitizeCells(strs))
}

// Flush writes the table to the underlying writer.
func (t *Table) Flush() error {
	all := t.rows
	if len(t.headers) > 0 {
		all = append([][]string{t.headers}, t.rows...)
	}
	t.rows = nil
	if len(all) == 0 {
		return nil
	}

	widths := columnWidths(all)
	if t.maxWidth > 0 {
		widths = fitColumns(widths, t.headers, t.maxWidth)
	}

	var b strings.Builder
	for _, row := range all {
		b.Reset()
		for i, cell := range row {
			if i > 0 {
				b.WriteString(strings.Repeat(" ", columnGap))
			}
			cell = truncateCell(cell, widths[i], t.ellipsis)
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-ansi.StringWidth(cell)))
			}
		}
		if _, err := fmt.Fprintln(t.out, strings.TrimRight(b.String(), " ")); err != nil {
			return err
		}
	}
	return nil
}

// truncateCell shortens cell to width display cells, ending in ellipsis.
// Whitespace before the ellipsis is dropped so "a very ..." reads "a very...".
func truncateCell(cell string, width int, ellipsis string) string {
	if ansi.StringWidth(cell) <= width {
		return cell
	}
	keep := width - ansi.StringWidth(ellipsis)
	if keep <= 0 {
		return ansi.Truncate(cell, width, "")
	}
	return strings.TrimRight(ansi.Truncate(cell, keep, ""), " ") + ellipsis
}

// sanitizeCells flattens embedded newlines and tabs so one record stays on one
// line and cannot break column alignment.
func sanitizeCells(cells []string) []string {
	out := make([]string, len(cells))
	for i, cell := range cells {
		out[i] = cellReplacer.Replace(cell)
	}
	return out
}

var cellReplacer = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")

func columnWidths(rows [][]string) []int {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			if w := ansi.StringWidth(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

// fitColumns narrows columns until the row fits maxWidth, one cell at a time
// from the currently widest truncatable column, so the long free-text column
// (NAME, TITLE, ...) gives way first and short columns are left intact. ID
// columns are never truncated: a cut identifier is useless. When nothing more
// can give, the row is left to wrap.
func fitColumns(widths []int, headers []string, maxWidth int) []int {
	fitted := append([]int(nil), widths...)
	total := columnGap * (len(fitted) - 1)
	for _, w := range fitted {
		total += w
	}
	over := total - maxWidth
	for over > 0 {
		widest := -1
		for i, w := range fitted {
			if isIDColumn(headers, i) || w <= minTruncatedWidth {
				continue
			}
			if widest < 0 || w > fitted[widest] {
				widest = i
			}
		}
		if widest < 0 {
			break
		}
		fitted[widest]--
		over--
	}
	return fitted
}

func isIDColumn(headers []string, index int) bool {
	if index >= len(headers) {
		return false
	}
	h := strings.ToUpper(strings.TrimSpace(headers[index]))
	return h == "ID" || strings.HasSuffix(h, " ID") || strings.HasSuffix(h, "_ID")
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
	t.SetEllipsis(w.Glyphs().Ellipsis)
	if w.IsTTY() {
		t.SetMaxWidth(terminalWidth(w.Out))
	}

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
