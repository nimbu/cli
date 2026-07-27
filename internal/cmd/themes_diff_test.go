package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func diffStylesContext(color string) context.Context {
	ctx := context.Background()
	return output.WithWriter(ctx, &output.Writer{
		Out:   &strings.Builder{},
		Err:   &strings.Builder{},
		NoTTY: true,
		Color: color,
	})
}

func TestDiffStylesColorizeEmitsANSIWhenColorAlways(t *testing.T) {
	styles := newDiffStyles(diffStylesContext("always"))
	if !styles.enabled {
		t.Fatal("expected styles to be enabled with --color=always")
	}

	diff := strings.Join([]string{
		"--- remote/layouts/default.liquid",
		"+++ local/layouts/default.liquid",
		"@@ -1,2 +1,2 @@",
		"-line two",
		"+line changed",
		" context",
		"",
	}, "\n")
	colored := styles.colorize(diff)

	if !strings.Contains(colored, "\x1b[") {
		t.Fatalf("expected ANSI escape codes in colored diff:\n%q", colored)
	}
	for _, want := range []string{
		"\x1b[32m+line changed",
		"\x1b[31m-line two",
		"\x1b[36m@@ -1,2 +1,2 @@",
		"\x1b[1m--- remote/layouts/default.liquid",
	} {
		if !strings.Contains(colored, want) {
			t.Fatalf("colored diff missing %q:\n%q", want, colored)
		}
	}
	if strings.Contains(colored, "\x1b[32m context") || strings.Contains(colored, "\x1b[31m context") {
		t.Fatalf("context lines should stay uncolored:\n%q", colored)
	}
}

func TestDiffStylesStatusLine(t *testing.T) {
	styles := newDiffStyles(diffStylesContext("always"))
	changed := styles.statusLine("changed", "layouts/default.liquid")
	if !strings.Contains(changed, "\x1b[33mchanged") {
		t.Fatalf("expected yellow changed status:\n%q", changed)
	}
	missing := styles.statusLine("missing", "snippets/header.liquid")
	if !strings.Contains(missing, "\x1b[31mmissing") {
		t.Fatalf("expected red missing status:\n%q", missing)
	}
	if !strings.Contains(changed, "\x1b[1mlayouts/default.liquid") {
		t.Fatalf("expected bold path:\n%q", changed)
	}
}

func TestDiffStylesDisabledWithoutColor(t *testing.T) {
	styles := newDiffStyles(diffStylesContext("never"))
	if styles.enabled {
		t.Fatal("expected styles to be disabled with --color=never")
	}
	diff := "-old\n+new\n"
	if got := styles.colorize(diff); got != diff {
		t.Fatalf("diff should pass through unchanged: %q", got)
	}
	if got := styles.statusLine("changed", "a.liquid"); got != "changed a.liquid" {
		t.Fatalf("status line should be plain: %q", got)
	}
}
