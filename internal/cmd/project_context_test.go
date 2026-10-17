package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

// stubInteractivePrompt makes prompts see a terminal (or not) on stdin and
// clears CI so the host runner does not leak into the test.
func stubInteractivePrompt(t *testing.T, terminal bool) {
	t.Helper()
	t.Setenv("CI", "")
	original := promptStdinIsTerminal
	promptStdinIsTerminal = func() bool { return terminal }
	t.Cleanup(func() { promptStdinIsTerminal = original })
}

func TestPromptBlocker(t *testing.T) {
	tests := []struct {
		name     string
		flags    *RootFlags
		terminal bool
		ci       string
		want     string
	}{
		{name: "interactive", flags: &RootFlags{}, terminal: true, want: ""},
		{name: "no-input", flags: &RootFlags{NoInput: true}, terminal: true, want: "--no-input"},
		{name: "piped stdin", flags: &RootFlags{}, terminal: false, want: "stdin is not a terminal"},
		{name: "CI", flags: &RootFlags{}, terminal: true, ci: "true", want: "CI"},
		{name: "CI=false is interactive", flags: &RootFlags{}, terminal: true, ci: "false", want: ""},
		{name: "CI=0 is interactive", flags: nil, terminal: true, ci: "0", want: ""},
		{name: "CI=no is interactive", flags: &RootFlags{}, terminal: true, ci: "no", want: ""},
		{name: "CI=NO is interactive", flags: &RootFlags{}, terminal: true, ci: "NO", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubInteractivePrompt(t, tt.terminal)
			t.Setenv("CI", tt.ci)
			if got := promptBlocker(tt.flags); got != tt.want {
				t.Fatalf("promptBlocker() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfirmPromptRefusesWithoutTerminal(t *testing.T) {
	stubInteractivePrompt(t, false)

	ok, err := confirmPrompt(&RootFlags{}, "delete 3 remote app files")
	if ok || err == nil {
		t.Fatalf("confirmPrompt() = %v, %v; want refusal", ok, err)
	}
	desc := classifyError(err)
	if desc.Code != errorConfirmationRequired || desc.ExitCode != ExitUsage {
		t.Fatalf("classified as %s exit %d", desc.Code, desc.ExitCode)
	}
	if !strings.Contains(desc.Message, "refusing to delete 3 remote app files without --force") ||
		!strings.Contains(desc.Message, "stdin is not a terminal") {
		t.Fatalf("message %q", desc.Message)
	}
	if !strings.Contains(desc.Hint, "--force") || !strings.Contains(desc.Hint, "--yes") {
		t.Fatalf("hint %q should name --force and --yes", desc.Hint)
	}
}

func TestConfirmPromptHonorsForceAndYes(t *testing.T) {
	stubInteractivePrompt(t, false)
	for _, flags := range []*RootFlags{{Force: true}, {Yes: true}} {
		ok, err := confirmPrompt(flags, "overwrite existing menu main")
		if !ok || err != nil {
			t.Fatalf("confirmPrompt(%+v) = %v, %v", flags, ok, err)
		}
	}
}

func TestConfirmThemeOverwriteRefusesInCI(t *testing.T) {
	stubInteractivePrompt(t, true)
	t.Setenv("CI", "true")
	var stderr bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: &bytes.Buffer{}, Err: &stderr, Color: "never"})

	ok, err := confirmThemeOverwrite(&RootFlags{})(ctx, themes.Resource{DisplayPath: "templates/page.liquid"}, &api.Error{StatusCode: 409})
	if ok || err == nil {
		t.Fatalf("confirm = %v, %v; want refusal", ok, err)
	}
	desc := classifyError(err)
	if desc.Code != errorConfirmationRequired || !strings.Contains(desc.Message, "templates/page.liquid") || !strings.Contains(desc.Message, "CI") {
		t.Fatalf("desc = %+v", desc)
	}
	if stderr.Len() != 0 {
		t.Fatalf("refusal must not print a prompt, got %q", stderr.String())
	}
}

func TestProtectedEnvironmentIgnoresForceAndNamesYes(t *testing.T) {
	stubInteractivePrompt(t, false)
	flags := &RootFlags{Environment: "production", SelectedEnvironmentProtected: true, Force: true}
	err := confirmProtectedEnvironment(context.Background(), flags, false, "release project")
	desc := classifyError(err)
	if desc.Code != errorConfirmationRequired || !strings.Contains(desc.Message, "requires --yes") || !strings.Contains(desc.Hint, "--force does not bypass a protected environment") {
		t.Fatalf("desc = %+v", desc)
	}
}

func TestRequireForceAndWriteAreTyped(t *testing.T) {
	desc := classifyError(requireForce(&RootFlags{}, "channel news"))
	if desc.Code != errorConfirmationRequired || desc.ExitCode != ExitUsage || desc.Hint == "" {
		t.Fatalf("requireForce desc = %+v", desc)
	}
	if err := requireForce(&RootFlags{Yes: true}, "channel news"); err != nil {
		t.Fatalf("--yes should satisfy requireForce: %v", err)
	}
	desc = classifyError(requireExactConfirm("", "news", "empty channel news"))
	if desc.Code != errorConfirmationRequired || desc.ExitCode != ExitUsage || !strings.Contains(desc.Hint, `--confirm "news"`) {
		t.Fatalf("requireExactConfirm desc = %+v", desc)
	}
	desc = classifyError(requireWrite(&RootFlags{Readonly: true}, "empty channel"))
	if desc.Code != errorReadonly || desc.ExitCode != ExitUsage || !strings.Contains(desc.Hint, "NIMBU_READONLY") {
		t.Fatalf("requireWrite desc = %+v", desc)
	}
}

func TestYesFlagParsesGloballyAndAfterSubcommand(t *testing.T) {
	for _, args := range [][]string{
		{"-y", "release", "--dry-run"},
		{"release", "--yes", "--dry-run"},
		{"schema", "apply", "-y"},
	} {
		parser, cli, err := newParser()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(args); err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		if !cli.Yes {
			t.Fatalf("parse %v: --yes not set", args)
		}
	}
}
