package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
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

func TestWriteThemeDiffPlainKeepsContentDiffOnOneRecord(t *testing.T) {
	out := &strings.Builder{}
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: out, Err: &strings.Builder{}, NoTTY: true})
	entries := []themes.DiffEntry{
		{Status: "modified", Path: "layouts/default.liquid", Diff: "--- remote/layouts/default.liquid\n+++ local/layouts/default.liquid\n@@ -1 +1 @@\n-\told\n+\tnew\n"},
		{Status: "missing", Path: "snippets/a.liquid"},
	}

	if err := writeThemeDiffPlain(ctx, entries, true); err != nil {
		t.Fatalf("write plain: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != len(entries) {
		t.Fatalf("got %d lines, want one per entry:\n%s", len(lines), out.String())
	}
	for i, line := range lines {
		if fields := strings.Split(line, "\t"); len(fields) != 3 {
			t.Fatalf("line %d has %d fields, want 3: %q", i, len(fields), line)
		}
	}
	if !strings.HasPrefix(lines[0], "modified\tlayouts/default.liquid\t--- remote/") || !strings.Contains(lines[0], `\n-\told\n`) {
		t.Fatalf("diff field not escaped: %q", lines[0])
	}

	out.Reset()
	if err := writeThemeDiffPlain(ctx, entries, false); err != nil {
		t.Fatalf("write plain: %v", err)
	}
	if got := out.String(); got != "modified\tlayouts/default.liquid\nmissing\tsnippets/a.liquid\n" {
		t.Fatalf("without --content got %q", got)
	}
}
