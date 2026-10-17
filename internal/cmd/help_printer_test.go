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

	"github.com/nimbu/cli/internal/output"
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

// Examples and verbatim Help() blocks are not re-flowed, so every rendered
// help line must fit the 80-column default (Usage lines are Kong's and may
// run long). Regression: `pages update --help` had an 88-column line.
func TestHelpFitsEightyColumns(t *testing.T) {
	pinHelpEnv(t)
	parser, _, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	var paths [][]string
	_ = kong.Visit(parser.Model.Node, func(v kong.Visitable, next kong.Next) error {
		node, ok := v.(*kong.Node)
		if !ok || node.Hidden {
			return next(nil)
		}
		var path []string
		for n := node; n != nil && n.Type == kong.CommandNode; n = n.Parent {
			path = append([]string{n.Name}, path...)
		}
		paths = append(paths, path)
		return next(nil)
	})
	if len(paths) < 100 {
		t.Fatalf("visited only %d commands", len(paths))
	}
	for _, path := range paths {
		out := helpOutput(t, append(path, "--help")...)
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "Usage:") {
				continue
			}
			if len(line) > defaultHelpWidth {
				t.Errorf("%v --help: line wider than %d columns (%d): %q", path, defaultHelpWidth, len(line), line)
			}
		}
	}
}

// exampleArgs extracts the nimbu arguments from an example shell line.
func exampleArgs(line string) []string {
	at := strings.Index(line, "nimbu ")
	if at < 0 {
		return nil
	}
	rest := line[at+len("nimbu "):]
	for _, stop := range []string{" | ", " < ", `)"`} {
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

// Regression: `nimbu --verbose` exited 2 with "expected one of ..." while
// bare `nimbu` printed help; bare `nimbu api` failed on the hidden legacy
// command's flags and hinted at `nimbu api legacy --help`.
func TestImplicitHelpForRootFlagsAndHiddenDefault(t *testing.T) {
	pinHelpEnv(t)
	root := helpOutput(t, "--help")
	for _, args := range [][]string{{"--verbose"}, {"--site", "demo"}} {
		if got := helpOutput(t, args...); got != root {
			t.Fatalf("%v should print root help:\n%s", args, got)
		}
	}
	if got, want := helpOutput(t, "api"), helpOutput(t, "api", "--help"); got != want {
		t.Fatalf("bare api should print the api group help:\n%s", got)
	}
	code, _, stderr := captureExecute(t, []string{"api", "--method", "GET"})
	if code != ExitUsage || !strings.Contains(stderr, "run `nimbu api --help` for usage") || strings.Contains(stderr, "legacy") {
		t.Fatalf("api --method GET: exit %d stderr=%q", code, stderr)
	}
}

// Regression: an implicit help request in --json/--plain mode printed text
// help on stdout with exit 0, breaking `| jq`. It stays a usage error with
// the JSON envelope; an explicit help request still prints help.
func TestImplicitHelpStaysUsageErrorInMachineMode(t *testing.T) {
	pinHelpEnv(t)
	for _, args := range [][]string{{"channels", "--json"}, {"--json"}, {"api", "--json"}} {
		code, stdout, stderr := captureExecute(t, args)
		if code != ExitUsage || stdout != "" || !strings.Contains(stderr, `"code": "cli.usage.invalid_args"`) {
			t.Fatalf("%v: exit %d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
	code, stdout, _ := captureExecute(t, []string{"channels", "--plain"})
	if code != ExitUsage || stdout != "" {
		t.Fatalf("channels --plain: exit %d stdout=%q", code, stdout)
	}

	t.Setenv("NIMBU_JSON", "1")
	code, stdout, stderr := captureExecute(t, []string{"channels"})
	if code != ExitUsage || stdout != "" || !strings.Contains(stderr, `"code": "cli.usage.invalid_args"`) {
		t.Fatalf("NIMBU_JSON=1 channels: exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if got := helpOutput(t, "channels", "--json=false"); !strings.HasPrefix(got, "Usage: nimbu channels <command>") {
		t.Fatalf("--json=false should print help:\n%s", got)
	}
	if got := helpOutput(t, "help", "channels"); !strings.HasPrefix(got, "Usage: nimbu channels <command>") {
		t.Fatalf("explicit help in JSON mode should print help:\n%s", got)
	}
}

// Regression: `nimbu channels help` and `nimbu help help` exited 2 with
// "unexpected argument help".
func TestHelpWordInsideGroupPrintsHelp(t *testing.T) {
	pinHelpEnv(t)
	if got, want := helpOutput(t, "channels", "help"), helpOutput(t, "channels", "--help"); got != want {
		t.Fatalf("channels help differs from channels --help:\n%s", got)
	}
	if got, want := helpOutput(t, "channels", "help", "list"), helpOutput(t, "channels", "list", "--help"); got != want {
		t.Fatalf("channels help list differs from channels list --help:\n%s", got)
	}
	root := helpOutput(t, "--help")
	if got := helpOutput(t, "help", "help"); got != root || !strings.Contains(got, `nimbu help <command>`) {
		t.Fatalf("help help should print root help naming `nimbu help <command>`:\n%s", got)
	}
	// Groups with a default subcommand: `nimbu api` runs the hidden legacy
	// form, `nimbu auth keyring` runs show.
	for _, group := range [][]string{{"api"}, {"auth", "keyring"}} {
		want := helpOutput(t, append(append([]string{}, group...), "--help")...)
		if got := helpOutput(t, append(append([]string{}, group...), "help")...); got != want {
			t.Fatalf("%v help differs from %v --help:\n%s", group, group, got)
		}
	}
	for _, args := range [][]string{{"channels", "help"}, {"help", "help"}, {"api", "help"}, {"auth", "keyring", "help"}} {
		code, _, stderr := captureExecute(t, args)
		if code != 0 {
			t.Fatalf("%v: exit %d stderr=%q", args, code, stderr)
		}
	}
}

func TestRewriteHelpArgs(t *testing.T) {
	pinHelpEnv(t)
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
		{[]string{"api"}, []string{"api", "--help"}}, // hidden default subcommand
		{[]string{"api", "--method", "GET"}, []string{"api", "--method", "GET"}},
		{[]string{"auth", "keyring"}, []string{"auth", "keyring"}}, // visible default: runs show
		{[]string{"server"}, []string{"server"}},                   // leaf
		// The root with only global flags prints root help.
		{[]string{"--verbose"}, []string{"--verbose", "--help"}},
		{[]string{"--site", "demo"}, []string{"--site", "demo", "--help"}},
		{[]string{"--site"}, []string{"--site"}},
		{[]string{"--version"}, []string{"--version"}},
		{[]string{"--verbose", "--version"}, []string{"--verbose", "--version"}},
		// Machine output keeps the usage error for implicit help requests.
		{[]string{"--json"}, []string{"--json"}},
		{[]string{"channels", "--json"}, []string{"channels", "--json"}},
		{[]string{"channels", "--plain"}, []string{"channels", "--plain"}},
		{[]string{"api", "--json"}, []string{"api", "--json"}},
		{[]string{"channels", "--json=false"}, []string{"channels", "--json=false", "--help"}},
		{[]string{"channels", "help", "--json"}, []string{"channels", "--json", "--help"}},
		{[]string{"channels", "--", "x"}, []string{"channels", "--", "x"}},
		// A trailing value flag keeps Kong's "expected value" error.
		{[]string{"channels", "--site"}, []string{"channels", "--site"}},
		{[]string{"help", "channels", "--site"}, []string{"channels", "--site", "--help"}},
		{[]string{"translations", "get", "help"}, []string{"translations", "get", "help"}},
		// "help" as a word inside a group, and `help help`.
		{[]string{"help", "help"}, []string{"--help"}},
		{[]string{"channels", "help"}, []string{"channels", "--help"}},
		{[]string{"channels", "entries", "help"}, []string{"channels", "entries", "--help"}},
		{[]string{"channels", "help", "list"}, []string{"channels", "list", "--help"}},
		{[]string{"help", "channels", "help"}, []string{"channels", "--help"}},
		{[]string{"--site", "demo", "channels", "help"}, []string{"--site", "demo", "channels", "--help"}},
		{[]string{"channels", "list", "help"}, []string{"channels", "list", "help"}}, // leaf: Kong decides
		{[]string{"api", "help"}, []string{"api", "--help"}},                         // default takes no positionals
		{[]string{"auth", "keyring", "help"}, []string{"auth", "keyring", "--help"}},
		{[]string{"auth", "keyring", "help", "set"}, []string{"auth", "keyring", "set", "--help"}},
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

// Help must not color machine output, whether --json/--plain come from args
// or the environment; "auto" (explicit or default) yields to them, while an
// explicit always/never still wins.
func TestHelpColorModeHonorsMachineOutput(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"default", nil, nil, "auto"},
		{"--json", []string{"--json"}, nil, "never"},
		{"--plain", []string{"--plain"}, nil, "never"},
		{"--json=true", []string{"--json=true"}, nil, "never"},
		{"--json=false", []string{"--json=false"}, nil, "auto"},
		{"explicit auto with --json", []string{"--json", "--color=auto"}, nil, "never"},
		{"NIMBU_JSON=1", nil, map[string]string{"NIMBU_JSON": "1"}, "never"},
		{"NIMBU_PLAIN=true", nil, map[string]string{"NIMBU_PLAIN": "true"}, "never"},
		{"NIMBU_JSON=0", nil, map[string]string{"NIMBU_JSON": "0"}, "auto"},
		{"--json=false beats NIMBU_JSON=1", []string{"--json=false"}, map[string]string{"NIMBU_JSON": "1"}, "auto"},
		{"--plain=false beats NIMBU_PLAIN=true", []string{"--plain=false"}, map[string]string{"NIMBU_PLAIN": "true"}, "auto"},
		{"last --json wins", []string{"--json", "--json=false"}, nil, "auto"},
		{"--json=false keeps NIMBU_PLAIN", []string{"--json=false"}, map[string]string{"NIMBU_PLAIN": "1"}, "never"},
		{"--json after --", []string{"--", "--json"}, nil, "auto"},
		{"--color=always beats --json", []string{"--json", "--color=always"}, nil, "always"},
		{"NIMBU_COLOR=never", nil, map[string]string{"NIMBU_COLOR": "never"}, "never"},
		{"--no-input is not a color signal", []string{"--no-input"}, nil, "auto"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"NIMBU_COLOR", "NIMBU_JSON", "NIMBU_PLAIN"} {
				t.Setenv(key, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := helpColorMode(tc.args); got != tc.want {
				t.Fatalf("helpColorMode(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// Help colors come from the shared palette and follow the stdout profile.
func TestHelpStyleUsesSharedPaletteAndProfile(t *testing.T) {
	style := newHelpStyle(termenv.ANSI256)
	for _, got := range []string{style.usage("Usage:"), style.section("Flags:"), style.command("list"), style.flag("--json")} {
		if strings.Contains(got, "38;2;") || !strings.Contains(got, "38;5;") {
			t.Fatalf("256-color help emitted %q", got)
		}
	}
	truecolor := newHelpStyle(termenv.TrueColor)
	if got := truecolor.flag("--json"); got != output.NewTheme(termenv.TrueColor).Muted("--json") {
		t.Fatalf("help flags should use the shared Muted token, got %q", got)
	}
	if got := newHelpStyle(termenv.Ascii).usage("Usage:"); got != "Usage:" {
		t.Fatalf("no color must leave help plain, got %q", got)
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
