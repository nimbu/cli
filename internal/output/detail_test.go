package output

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteDetail(t *testing.T) {
	var buf bytes.Buffer
	fields := []Field{
		FAlways("ID", "abc-123"),
		FAlways("Name", "Widget"),
		F("SKU", "WDG-01"),
		F("Description", ""),
		FAlways("Stock", 0),
	}

	if err := WriteDetail(&buf, fields); err != nil {
		t.Fatal(err)
	}

	got := buf.String()

	// FAlways fields should always appear
	detailAssertContains(t, got, "ID:")
	detailAssertContains(t, got, "Name:")
	detailAssertContains(t, got, "Stock:")

	// F with non-zero should appear
	detailAssertContains(t, got, "SKU:")

	// F with zero-value string should be skipped
	detailAssertNotContains(t, got, "Description:")
}

func TestWriteDetailAllSkipped(t *testing.T) {
	var buf bytes.Buffer
	fields := []Field{
		F("Empty", ""),
		F("Zero", 0),
		F("Nil", nil),
	}

	if err := WriteDetail(&buf, fields); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); got != "" {
		t.Fatalf("expected empty output, got %q", got)
	}
}

func TestDetailJSON(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()
	ctx = WithMode(ctx, Mode{JSON: true})
	ctx = WithWriter(ctx, &Writer{Out: &buf, Err: &buf, NoTTY: true})

	data := map[string]string{"id": "123", "name": "Test"}
	err := Detail(ctx, data, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %s", err)
	}
	if got["id"] != "123" {
		t.Fatalf("expected id=123, got %s", got["id"])
	}
}

func TestDetailPlain(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()
	ctx = WithMode(ctx, Mode{Plain: true})
	ctx = WithWriter(ctx, &Writer{Out: &buf, Err: &buf, NoTTY: true})

	err := Detail(ctx, nil, []any{"123", "test", "active"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); got != "123\ttest\tactive\n" {
		t.Fatalf("got %q", got)
	}
}

func TestDetailHuman(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()
	ctx = WithMode(ctx, Mode{})
	ctx = WithWriter(ctx, &Writer{Out: &buf, Err: &buf, NoTTY: true})

	fields := []Field{
		FAlways("ID", "abc"),
		FAlways("Name", "Widget"),
		F("Empty", ""),
	}

	err := Detail(ctx, nil, nil, fields)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	detailAssertContains(t, got, "ID:")
	detailAssertContains(t, got, "Name:")
	detailAssertNotContains(t, got, "Empty:")
}

func TestWriteDetailSliceAndMap(t *testing.T) {
	var buf bytes.Buffer
	fields := []Field{
		F("Tags", []string{"a", "b"}),
		F("EmptyTags", []string(nil)),
		F("Meta", map[string]string{"k": "v"}),
		F("EmptyMeta", map[string]string(nil)),
	}

	if err := WriteDetail(&buf, fields); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	detailAssertContains(t, got, "Tags:")
	detailAssertNotContains(t, got, "EmptyTags:")
	detailAssertContains(t, got, "Meta:")
	detailAssertNotContains(t, got, "EmptyMeta:")
}

func TestDetailBoolAndFloat(t *testing.T) {
	var buf bytes.Buffer
	fields := []Field{
		FAlways("Digital", true),
		FAlways("Disabled", false),
		F("Price", 12.50),
		F("Discount", 0.0),
	}

	if err := WriteDetail(&buf, fields); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	detailAssertContains(t, got, "Digital:")
	detailAssertContains(t, got, "Disabled:")
	detailAssertContains(t, got, "Price:")
	detailAssertNotContains(t, got, "Discount:")
}

func detailAssertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, s)
	}
}

func detailAssertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected output to NOT contain %q, got:\n%s", substr, s)
	}
}
