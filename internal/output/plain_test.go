package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlainWritesTSV(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{Plain: true})

	if err := Plain(ctx, "abc", 42, true); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if got != "abc\t42\ttrue\n" {
		t.Fatalf("expected TSV line, got %q", got)
	}
}

func TestPlainSingleValue(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{Plain: true})

	if err := Plain(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "hello\n" {
		t.Fatalf("expected %q, got %q", "hello\n", got)
	}
}

func TestPlainRows(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{Plain: true})

	rows := [][]any{
		{"a", 1},
		{"b", 2},
	}
	if err := PlainRows(ctx, rows); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), out.String())
	}
	if lines[0] != "a\t1" {
		t.Errorf("line 0: got %q", lines[0])
	}
	if lines[1] != "b\t2" {
		t.Errorf("line 1: got %q", lines[1])
	}
}

type testItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestPlainFromStruct(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{Plain: true})

	item := testItem{ID: "1", Name: "Alice", Age: 30}
	if err := PlainFromStruct(ctx, item, []string{"id", "name"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "1\tAlice\n" {
		t.Fatalf("expected %q, got %q", "1\tAlice\n", got)
	}
}

func TestPlainFromSlice(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{Plain: true})

	items := []testItem{
		{ID: "1", Name: "Alice", Age: 30},
		{ID: "2", Name: "Bob", Age: 25},
	}
	if err := PlainFromSlice(ctx, items, []string{"id", "name"}); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "1\tAlice" {
		t.Errorf("line 0: got %q", lines[0])
	}
}

func TestPlainFromStructUnknownField(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{Plain: true})

	item := testItem{ID: "1", Name: "Alice"}
	err := PlainFromStruct(ctx, item, []string{"id", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the bad field name, got: %v", err)
	}
}
