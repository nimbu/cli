package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func colorDecisionContext(color string) context.Context {
	var stdout, stderr bytes.Buffer
	return output.WithWriter(context.Background(), &output.Writer{Out: &stdout, Err: &stderr, Color: color})
}

// Each human renderer must follow the shared per-stream color decision:
// --color=always forces escapes even off a terminal, NO_COLOR beats auto,
// and a non-terminal stream in auto mode stays plain.
func TestHumanRenderersFollowSharedColorDecision(t *testing.T) {
	type renderer struct {
		name   string
		render func(ctx context.Context) string
	}
	renderers := []renderer{
		{name: "themes diff", render: func(ctx context.Context) string {
			return newDiffStyles(ctx).colorize("+added\n-removed")
		}},
		{name: "channel fields", render: func(ctx context.Context) string {
			return newChannelFieldPresenter(ctx).fieldType("string")
		}},
		{name: "site copy prompt", render: func(ctx context.Context) string {
			return newSiteCopyPromptStyle(output.WriterFromContext(ctx)).dim("hint")
		}},
	}

	for _, r := range renderers {
		t.Run(r.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", "")
			t.Setenv("TERM", "xterm-256color")
			if got := r.render(colorDecisionContext("always")); !strings.Contains(got, "\x1b[") {
				t.Fatalf("--color=always: expected escapes, got %q", got)
			}
			if got := r.render(colorDecisionContext("auto")); strings.Contains(got, "\x1b[") {
				t.Fatalf("auto off a terminal: expected plain text, got %q", got)
			}
			if got := r.render(colorDecisionContext("never")); strings.Contains(got, "\x1b[") {
				t.Fatalf("--color=never: expected plain text, got %q", got)
			}
		})
	}
}
