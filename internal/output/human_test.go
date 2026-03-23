package output

import (
	"bytes"
	"testing"
)

func TestFprintf(t *testing.T) {
	var buf bytes.Buffer
	ctx := testContextWithMode(&buf, &buf, Mode{})

	n, err := Fprintf(ctx, "hello %s %d", "world", 42)
	if err != nil {
		t.Fatal(err)
	}
	if n != 14 {
		t.Fatalf("expected 14 bytes, got %d", n)
	}
	if got := buf.String(); got != "hello world 42" {
		t.Fatalf("got %q", got)
	}
}

func TestFprintln(t *testing.T) {
	var buf bytes.Buffer
	ctx := testContextWithMode(&buf, &buf, Mode{})

	_, err := Fprintln(ctx, "line one")
	if err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "line one\n" {
		t.Fatalf("got %q", got)
	}
}

func TestFprintfUsesContextWriter(t *testing.T) {
	var buf bytes.Buffer
	ctx := testContextWithMode(&buf, &buf, Mode{})

	if _, err := Fprintf(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := Fprintln(ctx, "b"); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); got != "ab\n" {
		t.Fatalf("expected %q, got %q", "ab\n", got)
	}
}
