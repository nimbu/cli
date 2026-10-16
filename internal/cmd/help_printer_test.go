package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/muesli/termenv"
)

// pinHelpEnv makes help output deterministic: 80 columns, no color, no
// user config.
func pinHelpEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("COLUMNS", "80")
	t.Setenv("NO_COLOR", "1")
	for _, key := range []string{"NIMBU_COLOR", "NIMBU_SITE", "NIMBU_JSON", "NIMBU_PLAIN", "NIMBU_NO_INPUT"} {
		t.Setenv(key, "") // registers the restore
		_ = os.Unsetenv(key)
	}
}

func helpOutput(t *testing.T, args ...string) string {
	t.Helper()
	code, stdout, stderr := captureExecute(t, args)
	if code != 0 {
		t.Fatalf("%v: exit %d stderr=%q stdout=%q", args, code, stderr, stdout)
	}
	if stderr != "" {
		t.Fatalf("%v: unexpected stderr %q", args, stderr)
	}
	return stdout
}

// Set UPDATE_GOLDEN=1 to rewrite the help goldens after an intended change.
func assertHelpGolden(t *testing.T, name, got string) {
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(filepath.Join("testdata", name), []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	assertGolden(t, name, got)
}

func TestHelpGoldens(t *testing.T) {
	cases := []struct {
		golden string
		args   []string
	}{
		{"help_root.golden", []string{"--help"}},
		{"help_channels.golden", []string{"channels", "--help"}},
		{"help_channels_delete.golden", []string{"channels", "delete", "--help"}},
		{"help_channels_entries_update.golden", []string{"channels", "entries", "update", "--help"}},
		{"help_channels_entries_gallery_remove.golden", []string{"channels", "entries", "gallery", "remove", "--help"}},
		{"help_channels_delete_all.golden", []string{"channels", "delete", "--help-all"}},
	}
	for _, tc := range cases {
		t.Run(tc.golden, func(t *testing.T) {
			pinHelpEnv(t)
			assertHelpGolden(t, tc.golden, helpOutput(t, tc.args...))
		})
	}
}

// H1: a wrapped flag description stays with its flag, and every
// application-level flag (--env included) is listed under Global Flags.
func TestLeafHelpKeepsWrappedLinesWithTheirFlag(t *testing.T) {
	pinHelpEnv(t)
	out := helpOutput(t, "channels", "delete", "--help")

	flags, global := sectionText(t, out, "Flags:"), sectionText(t, out, "Global Flags:")
	if strings.Contains(flags, "$NIMBU_SITE") || strings.Contains(flags, "--env") {
		t.Fatalf("global flag text leaked into Flags:\n%s", flags)
	}
	for _, needle := range []string{"--env=STRING", "--site=STRING", "($NIMBU_SITE)"} {
		if !strings.Contains(global, needle) {
			t.Fatalf("Global Flags missing %q:\n%s", needle, global)
		}
	}
	siteAt := strings.Index(global, "--site=STRING")
	envVarAt := strings.Index(global, "($NIMBU_SITE)")
	jsonAt := strings.Index(global, "--json")
	if siteAt >= envVarAt || envVarAt >= jsonAt {
		t.Fatalf("($NIMBU_SITE) is not attached to --site:\n%s", global)
	}
}

// H2: destructive commands show --force (and --dry-run) by default.
func TestDestructiveHelpShowsSafetyFlags(t *testing.T) {
	pinHelpEnv(t)
	safety := sectionText(t, helpOutput(t, "channels", "delete", "--help"), "Safety:")
	if !strings.Contains(safety, "--force") {
		t.Fatalf("channels delete: Safety missing --force:\n%s", safety)
	}

	gallery := helpOutput(t, "channels", "entries", "gallery", "remove", "--help")
	safety = sectionText(t, gallery, "Safety:")
	for _, needle := range []string{"--force", "--dry-run"} {
		if !strings.Contains(safety, needle) {
			t.Fatalf("gallery remove: Safety missing %q:\n%s", needle, safety)
		}
	}
	if strings.Contains(sectionText(t, gallery, "Flags:"), "--dry-run") {
		t.Fatalf("--dry-run listed twice:\n%s", gallery)
	}

	all := helpOutput(t, "channels", "delete", "--help-all")
	if n := strings.Count(all, "--force "); n != 1 {
		t.Fatalf("--help-all lists --force %d times, want 1:\n%s", n, all)
	}

	list := helpOutput(t, "channels", "list", "--help")
	if strings.Contains(list, "Safety:") || strings.Contains(list, "--force") {
		t.Fatalf("non-destructive command shows safety flags:\n%s", list)
	}
}

// H4: root help is sectioned and uses no leader dots; every visible
// top-level command belongs to a known section.
func TestRootHelpSectionsCoverEveryCommand(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, s := range rootHelpSections {
		known[s.Key] = true
	}
	for _, child := range parser.Model.Children {
		if child.Hidden {
			continue
		}
		if key := commandHelpSection(child); !known[key] {
			t.Errorf("top-level command %q has section %q; add a known section:\"...\" tag", child.Name, key)
		}
	}

	// Order names real top-level commands of that section, so a rename
	// cannot silently push a command to the end of its section.
	for _, s := range rootHelpSections {
		for _, name := range s.Order {
			child := findChildCommand(parser.Model.Node, name)
			if child == nil || commandHelpSection(child) != s.Key {
				t.Errorf("section %q orders %q, which is not a command in that section", s.Key, name)
			}
		}
	}

	pinHelpEnv(t)
	out := helpOutput(t, "--help")
	if strings.Contains(out, "·") {
		t.Fatalf("root help still has leader dots:\n%s", out)
	}
	if strings.Contains(out, "Other commands:") {
		t.Fatalf("root help has unsectioned commands:\n%s", out)
	}
}

// H4: help sections live in the help renderer only; the machine contract
// has no help pseudo-command and no section data.
func TestCommandsContractUnaffectedByHelp(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	contract := buildCommandContract(parser.Model)
	for _, command := range contract.Commands {
		if command.Path == "nimbu help" || strings.HasPrefix(command.Path, "nimbu help ") {
			t.Fatalf("contract gained a help command: %+v", command)
		}
	}
	if contract.Commands[0].Path != "nimbu auth" {
		t.Fatalf("contract order changed: first command %q", contract.Commands[0].Path)
	}
}

// H5: the --help-all hint names the command, not its required flags.
func TestHelpAllFooterOmitsRequiredFlags(t *testing.T) {
	pinHelpEnv(t)
	out := helpOutput(t, "channels", "delete", "--help")
	if !strings.Contains(out, `Run "nimbu channels delete --help-all" for all flags.`) {
		t.Fatalf("footer wrong:\n%s", out)
	}
	if strings.Contains(out, "--channel=STRING --help-all") {
		t.Fatalf("footer echoes the required flag placeholder:\n%s", out)
	}
}

// H6: the most used commands carry examples.
func TestMostUsedCommandsHaveExamples(t *testing.T) {
	pinHelpEnv(t)
	for _, args := range [][]string{
		{},
		{"auth", "login"},
		{"sites", "list"},
		{"channels", "list"},
		{"channels", "entries", "list"},
		{"channels", "entries", "create"},
		{"channels", "entries", "update"},
		{"pages", "get"},
		{"pages", "update"},
		{"themes", "push"},
		{"themes", "sync"},
		{"themes", "pull"},
		{"server"},
		{"init"},
		{"release"},
		{"api"},
		{"completion"},
	} {
		out := helpOutput(t, append(args, "--help")...)
		start := strings.Index(out, "\nExamples:\n")
		if start < 0 {
			t.Errorf("%v: no Examples section:\n%s", args, out)
			continue
		}
		examples := out[start:]
		if end := strings.Index(examples, "\nRun \""); end >= 0 {
			examples = examples[:end]
		}
		if n := strings.Count(examples, "\n  # "); n < 2 {
			t.Errorf("%v: want at least 2 examples, got %d:\n%s", args, n, examples)
		}
	}
}

// H6: every example must parse with the current grammar.
func TestHelpExamplesParse(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	_ = kong.Visit(parser.Model.Node, func(v kong.Visitable, next kong.Next) error {
		node, ok := v.(*kong.Node)
		if !ok {
			return next(nil)
		}
		for _, example := range helpExamplesFor(node) {
			for _, line := range strings.Split(example.Command, "\n") {
				if len(helpIndent+line) > defaultHelpWidth {
					t.Errorf("%s: example line wider than %d columns: %q", node.FullPath(), defaultHelpWidth, line)
				}
			}
			command := strings.ReplaceAll(example.Command, "\\\n", " ")
			for _, line := range strings.Split(command, "\n") {
				args := exampleArgs(line)
				if args == nil {
					t.Errorf("%s: example has no nimbu invocation: %q", node.FullPath(), line)
					continue
				}
				fresh, _, err := newParser()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := fresh.Parse(args); err != nil {
					t.Errorf("%s: example %q does not parse: %v", node.FullPath(), line, err)
				}
				count++
			}
		}
		return next(nil)
	})
	if count < 30 {
		t.Fatalf("parsed only %d example lines", count)
	}
}

// exampleArgs extracts the nimbu arguments from an example shell line.
func exampleArgs(line string) []string {
	at := strings.Index(line, "nimbu ")
	if at < 0 {
		return nil
	}
	rest := line[at+len("nimbu "):]
	for _, stop := range []string{" | ", `)"`} {
		if i := strings.Index(rest, stop); i >= 0 {
			rest = rest[:i]
		}
	}
	var args []string
	var cur strings.Builder
	var quote rune
	inArg := false
	for _, r := range rest {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			cur.WriteRune(r)
		case r == '"' || r == '\'':
			quote, inArg = r, true
		case r == ' ':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args
}

// H3: `nimbu channels` prints the group's help on stdout and exits 0, and
// `nimbu help <command>` prints the same help.
func TestBareGroupAndHelpCommandPrintHelp(t *testing.T) {
	pinHelpEnv(t)
	want := helpOutput(t, "channels", "--help")

	if got := helpOutput(t, "channels"); got != want {
		t.Fatalf("bare group help differs:\n%s", got)
	}
	if got := helpOutput(t, "help", "channels"); got != want {
		t.Fatalf("help channels differs:\n%s", got)
	}
	if got, root := helpOutput(t, "help"), helpOutput(t, "--help"); got != root {
		t.Fatalf("nimbu help differs from nimbu --help:\n%s", got)
	}
	if got := helpOutput(t, "help", "channels", "entries"); !strings.HasPrefix(got, "Usage: nimbu channels entries <command>") {
		t.Fatalf("help channels entries:\n%s", got)
	}

	code, _, stderr := captureExecute(t, []string{"help", "bogus"})
	if code != ExitUsage || !strings.Contains(stderr, "bogus") {
		t.Fatalf("help bogus: exit %d stderr=%q", code, stderr)
	}
}

func TestRewriteHelpArgs(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in, want []string
	}{
		{[]string{"help"}, []string{"--help"}},
		{[]string{"help", "channels"}, []string{"channels", "--help"}},
		{[]string{"help", "channels", "entries"}, []string{"channels", "entries", "--help"}},
		{[]string{"--site", "demo", "help", "pages"}, []string{"--site", "demo", "pages", "--help"}},
		{[]string{"help", "channels", "--help-all", "--help"}, []string{"channels", "--help-all", "--help"}},
		{[]string{"channels"}, []string{"channels", "--help"}},
		{[]string{"channels", "entries"}, []string{"channels", "entries", "--help"}},
		{[]string{"--site=demo", "channels"}, []string{"--site=demo", "channels", "--help"}},
		{[]string{"mail"}, []string{"mail", "--help"}}, // alias
		{[]string{"channels", "--help"}, []string{"channels", "--help"}},
		// Untouched: complete commands, unknown words, values, defaults.
		{[]string{"channels", "list"}, []string{"channels", "list"}},
		{[]string{"channels", "bogus"}, []string{"channels", "bogus"}},
		{[]string{"--site", "channels", "pages"}, []string{"--site", "channels", "pages", "--help"}},
		{[]string{"api"}, []string{"api"}},       // default subcommand
		{[]string{"server"}, []string{"server"}}, // leaf
		{[]string{"channels", "--", "x"}, []string{"channels", "--", "x"}},
		// A trailing value flag keeps Kong's "expected value" error.
		{[]string{"channels", "--site"}, []string{"channels", "--site"}},
		{[]string{"help", "channels", "--site"}, []string{"channels", "--site", "--help"}},
		{[]string{"translations", "get", "help"}, []string{"translations", "get", "help"}},
		{[]string{}, []string{}},
	}
	for _, tc := range cases {
		got := rewriteHelpArgs(parser.Model, tc.in)
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("rewriteHelpArgs(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// H8: help colors follow the shared color policy for stdout.
func TestHelpColorProfileUsesSharedPolicy(t *testing.T) {
	var pipe bytes.Buffer
	cases := []struct {
		name    string
		args    []string
		env     map[string]string
		colored bool
	}{
		{"auto on a pipe", nil, nil, false},
		{"--color=always", []string{"--color=always"}, nil, true},
		{"--color always", []string{"--color", "always"}, nil, true},
		{"NIMBU_COLOR=always", nil, map[string]string{"NIMBU_COLOR": "always"}, true},
		{"flag beats env", []string{"--color=never"}, map[string]string{"NIMBU_COLOR": "always"}, false},
		{"--color=always beats NO_COLOR", []string{"--color=always"}, map[string]string{"NO_COLOR": "1"}, true},
		{"--json turns auto off", []string{"--json", "--color=auto"}, map[string]string{"FORCE_COLOR": "1"}, false},
		{"generic FORCE_COLOR ignored", nil, map[string]string{"FORCE_COLOR": "1", "CLICOLOR_FORCE": "1"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", "")
			t.Setenv("NIMBU_COLOR", "")
			t.Setenv("FORCE_COLOR", "")
			t.Setenv("CLICOLOR_FORCE", "")
			t.Setenv("TERM", "xterm-256color")
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			profile := helpColorProfile(tc.args, &pipe)
			if got := profile != termenv.Ascii; got != tc.colored {
				t.Fatalf("colored = %v (profile %v), want %v", got, profile, tc.colored)
			}
		})
	}
}

// Aliases are not part of the runnable path: `nimbu mail` hints must say
// "nimbu mails ...", not Kong's "nimbu mails (mail) ...".
func TestHelpHintsUseRunnableCommandPath(t *testing.T) {
	pinHelpEnv(t)
	out := helpOutput(t, "mail")
	for _, want := range []string{
		"Usage: nimbu mails <command> [flags]",
		`Run "nimbu mails <command> --help" for more information on a command.`,
		`Run "nimbu mails --help-all" for all flags.`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "(mail)") {
		t.Fatalf("alias leaked into usage or hints:\n%s", out)
	}
}

// On a narrow terminal descriptions stack under their flag instead of
// wrapping to a word per line, and no line exceeds the width.
func TestNarrowHelpStacksDescriptions(t *testing.T) {
	pinHelpEnv(t)
	t.Setenv("COLUMNS", "30")
	out := helpOutput(t, "channels", "delete", "--help")
	flags := sectionText(t, out, "Flags:")
	want := "Flags:\n      --channel=STRING\n        Channel ID or slug"
	if flags != want {
		t.Fatalf("narrow Flags section:\n%s\nwant:\n%s", flags, want)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Usage:") || strings.HasPrefix(line, "Run ") {
			continue
		}
		if len(line) > 30 {
			t.Errorf("line wider than 30 columns: %q", line)
		}
	}
}

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// Colors must not change the layout: stripped colored help equals plain help.
func TestColoredHelpMatchesPlainLayout(t *testing.T) {
	pinHelpEnv(t)
	plain := helpOutput(t, "channels", "delete", "--help")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	colored := helpOutput(t, "channels", "delete", "--help", "--color=always")
	if !strings.Contains(colored, "\x1b[") {
		t.Fatal("--color=always produced no color")
	}
	if got := ansiEscape.ReplaceAllString(colored, ""); got != plain {
		t.Fatalf("colored layout differs from plain:\n%s\n---\n%s", got, plain)
	}
}

// sectionText returns the lines of a titled help section.
func sectionText(t *testing.T, out, title string) string {
	t.Helper()
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if line != title {
			continue
		}
		end := i + 1
		for end < len(lines) && lines[end] != "" {
			end++
		}
		return strings.Join(lines[i:end], "\n")
	}
	t.Fatalf("no %q section in:\n%s", title, out)
	return ""
}
