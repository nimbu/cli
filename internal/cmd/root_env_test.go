package cmd

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/output"
)

func TestEmptyFlagEnvIsTreatedAsUnset(t *testing.T) {
	for _, name := range []string{
		"NIMBU_JSON", "NIMBU_PLAIN", "NIMBU_NO_INPUT", "NIMBU_READONLY",
		"NIMBU_NO_PROGRESS", "NIMBU_TIMEOUT", "NIMBU_COLOR", "NIMBU_SITE",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("NIMBU_API_URL", "  ")

	parser, cli, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	unsetEmptyFlagEnv(parser.Model)
	if _, err := parser.Parse([]string{"config", "path"}); err != nil {
		t.Fatalf("empty NIMBU_* env must parse, got %v", err)
	}
	if cli.JSON || cli.Plain || cli.NoInput || cli.Readonly || cli.NoProgress {
		t.Fatalf("empty env set a bool flag: %+v", cli.RootFlags)
	}
	if cli.Timeout != 30*time.Second || cli.Color != "auto" || cli.APIURL != "https://api.nimbu.io" {
		t.Fatalf("empty env changed defaults: timeout=%s color=%q api=%q", cli.Timeout, cli.Color, cli.APIURL)
	}
	if _, set := os.LookupEnv("NIMBU_JSON"); set {
		t.Fatal("NIMBU_JSON should be unset")
	}
}

func TestNonEmptyFlagEnvStillApplies(t *testing.T) {
	t.Setenv("NIMBU_NO_INPUT", "1")
	parser, cli, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	unsetEmptyFlagEnv(parser.Model)
	if _, err := parser.Parse([]string{"config", "path"}); err != nil {
		t.Fatal(err)
	}
	if !cli.NoInput {
		t.Fatal("NIMBU_NO_INPUT=1 should set --no-input")
	}
}

func TestColorFlagRejectsUnknownValues(t *testing.T) {
	for _, tc := range []struct {
		args []string
		env  string
	}{
		{args: []string{"--color=bogus", "config", "path"}},
	} {
		t.Setenv("NIMBU_COLOR", tc.env)
		parser, _, err := newParser()
		if err != nil {
			t.Fatal(err)
		}
		unsetEmptyFlagEnv(parser.Model)
		_, err = parser.Parse(tc.args)
		if err == nil || !strings.Contains(err.Error(), `must be one of "auto","always","never"`) {
			t.Fatalf("args %v env %q: err = %v", tc.args, tc.env, err)
		}
	}

	for _, value := range []string{"auto", "always", "never"} {
		parser, cli, err := newParser()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse([]string{"--color", value, "config", "path"}); err != nil || cli.Color != value {
			t.Fatalf("--color %s: color=%q err=%v", value, cli.Color, err)
		}
	}
}

func TestNoInputDoesNotDisableTerminalDetection(t *testing.T) {
	writer := newRootWriter(RootFlags{NoInput: true, Color: "always", NoProgress: true}, output.Mode{})
	if writer.NoTTY {
		t.Fatal("--no-input must not mark the streams as non-terminals")
	}
	if !writer.ErrUseColor() || !writer.NoSpin {
		t.Fatalf("writer = %+v; color and --no-progress must still apply", writer)
	}
}

func TestColorEnvIsNormalized(t *testing.T) {
	for value, want := range map[string]string{" ": "", "Never": "never", " ALWAYS ": "always", "auto": "auto"} {
		t.Setenv("NIMBU_COLOR", value)
		if warning := normalizeColorEnv(); warning != "" {
			t.Fatalf("NIMBU_COLOR=%q warned: %s", value, warning)
		}
		if got := os.Getenv("NIMBU_COLOR"); got != want {
			t.Fatalf("NIMBU_COLOR=%q normalized to %q, want %q", value, got, want)
		}
	}
	t.Setenv("NIMBU_COLOR", "1")
	if warning := normalizeColorEnv(); !strings.Contains(warning, "NIMBU_COLOR") {
		t.Fatalf("unknown NIMBU_COLOR must warn naming the variable, got %q", warning)
	}
	if _, set := os.LookupEnv("NIMBU_COLOR"); set {
		t.Fatal("unknown NIMBU_COLOR must fall back to auto")
	}
}

func TestOddColorEnvDoesNotBreakCommands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	for _, value := range []string{" ", "NEVER", "Never", "AUTO", "1", "true", "yes"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("NIMBU_COLOR", value)
			var err error
			stderr := captureStderr(t, func() {
				captureStdout(t, func() error {
					err = execute([]string{"completion", "zsh"})
					return nil
				})
			})
			if err != nil {
				t.Fatalf("NIMBU_COLOR=%q: %v", value, err)
			}
			known := colorModes[strings.ToLower(strings.TrimSpace(value))] || strings.TrimSpace(value) == ""
			if warned := strings.Contains(stderr, "NIMBU_COLOR"); warned == known {
				t.Fatalf("NIMBU_COLOR=%q: warned=%v stderr=%q", value, warned, stderr)
			}
		})
	}
}

func TestColorFlagIsCaseInsensitive(t *testing.T) {
	got := normalizeColorArgs([]string{"--color=Never", "--color", "ALWAYS", "config", "--", "--color=X"})
	want := []string{"--color=never", "--color", "always", "config", "--", "--color=X"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("normalizeColorArgs = %v, want %v", got, want)
	}
}

func TestColorFlagErrorPointsAtHelpAll(t *testing.T) {
	t.Setenv("NIMBU_COLOR", "")
	t.Setenv("NIMBU_JSON", "")
	var err error
	stderr := captureStderr(t, func() { err = execute([]string{"--color=bogus", "config", "path"}) })
	if desc := classifyError(err); desc.ExitCode != ExitUsage {
		t.Fatalf("exit = %d, err = %v", desc.ExitCode, err)
	}
	if !strings.Contains(stderr, "nimbu config path --help-all") {
		t.Fatalf("hint should point at --help-all, stderr = %q", stderr)
	}
}
