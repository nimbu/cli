package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewTableWithHeaders(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "ID", "NAME")
	tbl.Row("1", "Alice")
	tbl.Row("2", "Bob")
	if err := tbl.Flush(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(lines[0], "ID") || !strings.Contains(lines[0], "NAME") {
		t.Errorf("header missing expected columns: %q", lines[0])
	}
	if !strings.Contains(lines[1], "Alice") {
		t.Errorf("row 1 missing Alice: %q", lines[1])
	}
}

func TestFormatValueBool(t *testing.T) {
	if got := formatValue(true); got != "yes" {
		t.Errorf("true should be 'yes', got %q", got)
	}
	if got := formatValue(false); got != "no" {
		t.Errorf("false should be 'no', got %q", got)
	}
}

func TestFormatValueStringSlice(t *testing.T) {
	got := formatValue([]string{"a", "b", "c"})
	if got != "a, b, c" {
		t.Errorf("expected comma-joined, got %q", got)
	}
}

func TestFormatValueNil(t *testing.T) {
	if got := formatValue(nil); got != "" {
		t.Errorf("nil should be empty string, got %q", got)
	}
}

func TestFormatValueNilPointer(t *testing.T) {
	var s *string
	if got := formatValue(s); got != "" {
		t.Errorf("nil pointer should be empty string, got %q", got)
	}
}

func TestPrintDispatchesJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{JSON: true})

	data := map[string]string{"key": "val"}
	humanCalled := false
	err := Print(ctx, data, []any{"val"}, func() error {
		humanCalled = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if humanCalled {
		t.Error("humanFn should not be called in JSON mode")
	}

	var got map[string]string
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("expected valid JSON, got: %s", out.String())
	}
	if got["key"] != "val" {
		t.Errorf("expected key=val, got %v", got["key"])
	}
}

func TestPrintDispatchesPlain(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{Plain: true})

	humanCalled := false
	err := Print(ctx, nil, []any{"a", "b"}, func() error {
		humanCalled = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if humanCalled {
		t.Error("humanFn should not be called in Plain mode")
	}
	if got := out.String(); got != "a\tb\n" {
		t.Errorf("expected TSV, got %q", got)
	}
}

func TestPrintDispatchesHuman(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{})

	humanCalled := false
	err := Print(ctx, nil, nil, func() error {
		humanCalled = true
		_, err := Fprintf(ctx, "hello human\n")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !humanCalled {
		t.Error("humanFn should be called in Human mode")
	}
	if got := out.String(); got != "hello human\n" {
		t.Errorf("expected human output, got %q", got)
	}
}

func TestWriteTable(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{})

	items := []testItem{
		{ID: "1", Name: "Alice", Age: 30},
		{ID: "2", Name: "Bob", Age: 25},
	}
	err := WriteTable(ctx, items, []string{"id", "name"}, []string{"ID", "NAME"})
	if err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if !strings.Contains(got, "ID") || !strings.Contains(got, "NAME") {
		t.Errorf("missing headers in: %q", got)
	}
	if !strings.Contains(got, "Alice") || !strings.Contains(got, "Bob") {
		t.Errorf("missing row data in: %q", got)
	}
}
