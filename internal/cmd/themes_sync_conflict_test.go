package cmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

func TestConfirmThemeOverwriteReusesReaderAcrossPrompts(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "answers")
	if err != nil {
		t.Fatalf("create input: %v", err)
	}
	if _, err := input.WriteString("n\ny\n"); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("rewind input: %v", err)
	}

	originalStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() { os.Stdin = originalStdin })

	var stderr bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{
		Out:   &bytes.Buffer{},
		Err:   &stderr,
		Color: "never",
		NoTTY: true,
	})
	confirm := confirmThemeOverwrite(&RootFlags{})
	conflict := &api.Error{StatusCode: 409, Message: "Conflict (Peter edited article.liquid)"}

	first, err := confirm(ctx, themes.Resource{DisplayPath: "templates/article.liquid"}, conflict)
	if err != nil {
		t.Fatalf("first prompt: %v", err)
	}
	second, err := confirm(ctx, themes.Resource{DisplayPath: "templates/page.liquid"}, conflict)
	if err != nil {
		t.Fatalf("second prompt: %v", err)
	}

	if first || !second {
		t.Fatalf("unexpected answers: first=%v second=%v", first, second)
	}
	if got := stderr.String(); !strings.Contains(got, "skipping upload of templates/article.liquid") || !strings.Contains(got, "forcing upload of templates/page.liquid") {
		t.Fatalf("unexpected stderr: %q", got)
	}
}
