package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// /dev/null is a character device, which the old ModeCharDevice check took
// for a terminal: CI's usual stdin then reached the prompt and failed with a
// bare EOF instead of cli.confirmation_required.
func TestStdinIsTerminalFalseForDevNull(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = devNull.Close() }()
	original := os.Stdin
	os.Stdin = devNull
	t.Cleanup(func() { os.Stdin = original })

	if stdinIsTerminal() {
		t.Fatal("stdin from the null device must not count as a terminal")
	}
	t.Setenv("CI", "")
	if got := promptBlocker(&RootFlags{}); got != "stdin is not a terminal" {
		t.Fatalf("promptBlocker() = %q, want refusal for the null device", got)
	}
}

func TestHumanErrorPrintsCancellationWithoutSignal(t *testing.T) {
	var buf bytes.Buffer
	err := fmt.Errorf("worker stopped: %w", context.Canceled)
	desc := classifyError(err)
	if desc.ExitCode != ExitInterrupted {
		t.Fatalf("exit = %d, want %d", desc.ExitCode, ExitInterrupted)
	}
	writeHumanError(&output.Writer{Err: &buf, Color: "never"}, err, desc)
	if !strings.Contains(buf.String(), "error: worker stopped: context canceled") {
		t.Fatalf("a cancellation no signal explains must not be silent, got %q", buf.String())
	}
}

func TestSiteCopyConflictResolverRefusesWithoutTerminal(t *testing.T) {
	stubInteractivePrompt(t, false)
	resolve := siteCopyConflictResolver(&RootFlags{}, nil)
	_, err := resolve(context.Background(), migrate.ExistingContentPrompt{Type: "Pages"})
	desc := classifyError(err)
	if desc.Code != errorConfirmationRequired || desc.ExitCode != ExitUsage || !strings.Contains(desc.Hint, "--force") {
		t.Fatalf("resolver error %v classified as %+v", err, desc)
	}
}

func TestLinePromptsMarkActivePrompt(t *testing.T) {
	pr, pw := io.Pipe()
	defer func() { _ = pr.Close() }()
	result := make(chan string, 1)
	go func() {
		line, _ := promptWithReader(bufio.NewReader(pr), "")
		result <- line
	}()

	deadline := time.Now().Add(2 * time.Second)
	for activePrompts.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("promptWithReader must mark the prompt so Ctrl-C exits at once")
		}
		time.Sleep(time.Millisecond)
	}
	_, _ = pw.Write([]byte("answer\n"))
	if got := <-result; got != "answer" {
		t.Fatalf("prompt answer = %q", got)
	}
	if n := activePrompts.Load(); n != 0 {
		t.Fatalf("activePrompts = %d after the prompt returned", n)
	}
}

func TestForceGuardsOutsideHelpersAreTyped(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "out.bin")
	if err := os.WriteFile(existing, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]error{
		"download overwrite": writeDownloadResponse(context.Background(), strings.NewReader("new"), existing, false),
		"role relation shrink": requireRelationShrinks(&RootFlags{}, "r1", []roleRelationDiff{
			{Field: "customers", BeforeKnown: true, Projected: true, Before: 10, After: 1},
		}),
		"consent replace": (&SettingsConsentConfigReplaceCmd{File: "-"}).Run(context.Background(), &RootFlags{}),
	}
	for name, err := range cases {
		desc := classifyError(err)
		if desc.Code != errorConfirmationRequired || desc.ExitCode != ExitUsage || !strings.Contains(desc.Hint, "--force") {
			t.Errorf("%s: %v classified as %+v", name, err, desc)
		}
	}
	if err := requireRelationShrinks(&RootFlags{Yes: true}, "r1", []roleRelationDiff{
		{Field: "customers", BeforeKnown: true, Projected: true, Before: 10, After: 1},
	}); err != nil {
		t.Fatalf("--yes must confirm like --force: %v", err)
	}
}
