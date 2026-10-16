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
		{args: []string{"config", "path"}, env: "bogus"},
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
