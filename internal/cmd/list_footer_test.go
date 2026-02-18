package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestWriteListFooterHuman(t *testing.T) {
	buf := &bytes.Buffer{}
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: buf, Err: buf})
	ctx = output.WithMode(ctx, output.Mode{})

	meta := listFooterMeta{Page: 1, PerPage: 25, Returned: 25, Total: 100, TotalKnown: true}
	if err := writeListFooter(ctx, "products", meta); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "Showing 25 of 100 products") {
		t.Fatalf("unexpected footer: %q", out)
	}
}

func TestWriteListFooterSkipsJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: buf, Err: buf})
	ctx = output.WithMode(ctx, output.Mode{JSON: true})

	meta := listFooterMeta{Page: 1, PerPage: 25, Returned: 25, Total: 100, TotalKnown: true}
	if err := writeListFooter(ctx, "products", meta); err != nil {
		t.Fatal(err)
	}

	if buf.Len() != 0 {
		t.Fatalf("expected no footer in json mode, got %q", buf.String())
	}
}
