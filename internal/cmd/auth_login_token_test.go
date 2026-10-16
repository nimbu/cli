package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func withStdinContent(t *testing.T, content string) {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = file
	t.Cleanup(func() {
		os.Stdin = original
		_ = file.Close()
	})
}

func tokenTestContext() (context.Context, *bytes.Buffer) {
	var stderr bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: &bytes.Buffer{}, Err: &stderr, Color: "never"})
	return ctx, &stderr
}

func TestReadTokenFromStdin(t *testing.T) {
	withLoginTerminal(t, false)
	withStdinContent(t, "  secret-token\n")
	ctx, stderr := tokenTestContext()

	token, err := (&AuthLoginCmd{Token: "-"}).readToken(ctx, &RootFlags{NoInput: true})
	if err != nil || token != "secret-token" {
		t.Fatalf("readToken() = %q, %v", token, err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stdin token must not warn, got %q", stderr.String())
	}
}

func TestReadTokenRejectsEmptyStdin(t *testing.T) {
	withLoginTerminal(t, false)
	withStdinContent(t, "\n")
	ctx, _ := tokenTestContext()

	_, err := (&AuthLoginCmd{Token: "-"}).readToken(ctx, &RootFlags{})
	desc := classifyError(err)
	if desc.Code != errorUsageInvalid || desc.ExitCode != ExitUsage || !strings.Contains(desc.Hint, "--token -") {
		t.Fatalf("desc = %+v", desc)
	}
}

func TestReadTokenPromptsHiddenOnTerminal(t *testing.T) {
	withLoginTerminal(t, true)
	original := promptTokenSecret
	t.Cleanup(func() { promptTokenSecret = original })
	promptTokenSecret = func(string) (string, error) { return "typed-token", nil }
	ctx, _ := tokenTestContext()

	token, err := (&AuthLoginCmd{Token: "-"}).readToken(ctx, &RootFlags{})
	if err != nil || token != "typed-token" {
		t.Fatalf("readToken() = %q, %v", token, err)
	}

	promptTokenSecret = func(string) (string, error) { return "", errors.New("must not prompt") }
	if _, err := (&AuthLoginCmd{Token: "-"}).readToken(ctx, &RootFlags{NoInput: true}); classifyError(err).Code != errorUsageInvalid {
		t.Fatalf("--no-input on a terminal should be a usage error, got %v", err)
	}
}

func TestReadTokenWarnsOnLiteralToken(t *testing.T) {
	ctx, stderr := tokenTestContext()
	token, err := (&AuthLoginCmd{Token: "literal-token"}).readToken(ctx, &RootFlags{})
	if err != nil || token != "literal-token" {
		t.Fatalf("readToken() = %q, %v", token, err)
	}
	if !strings.HasPrefix(stderr.String(), "warning: ") || !strings.Contains(stderr.String(), "--token -") {
		t.Fatalf("stderr = %q, want a warning recommending --token -", stderr.String())
	}
	if strings.Contains(stderr.String(), "literal-token") {
		t.Fatal("warning must not echo the token")
	}
}
