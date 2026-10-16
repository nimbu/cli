package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
)

// helpCommandName is the pseudo-command behind `nimbu help [<command>...]`.
// It is not part of the Kong model, so `nimbu commands` stays unchanged.
const helpCommandName = "help"

// rewriteHelpArgs turns help requests Kong would reject into --help:
//
//   - `nimbu help [<command>...]` becomes `nimbu [<command>...] --help`
//   - a bare command group (`nimbu channels`) gets --help, so it prints the
//     group's help and exits 0 instead of a usage error, like git and gh.
//
// Positional tokens are matched against the model; flags are skipped (with
// their value when the flag takes one). Anything Kong would reject anyway is
// left untouched so the normal error is reported.
func rewriteHelpArgs(app *kong.Application, args []string) []string {
	node := app.Node
	scope := flagScope{}
	scope.add(node)
	helpIndex := -1
	matched := 0
	unmatched := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			unmatched = true
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			if scope.takesValue(arg) {
				i++
			}
			continue
		}
		if node == app.Node && matched == 0 && helpIndex < 0 && arg == helpCommandName {
			helpIndex = i
			continue
		}
		child := findChildCommand(node, arg)
		if child == nil {
			unmatched = true
			break
		}
		node = child
		scope.add(node)
		matched++
	}

	out := args
	if helpIndex >= 0 {
		out = make([]string, 0, len(args))
		out = append(out, args[:helpIndex]...)
		out = append(out, args[helpIndex+1:]...)
	}
	if helpIndex >= 0 || (!unmatched && isCommandGroup(node)) {
		if !hasHelpFlag(out) {
			out = append(append([]string{}, out...), "--help")
		}
	}
	return out
}

// isCommandGroup reports whether node only dispatches to subcommands, so
// running it without one can only mean "show me what is in here".
func isCommandGroup(node *kong.Node) bool {
	if node == nil || node.Type != kong.CommandNode || node.DefaultCmd != nil || len(node.Positional) > 0 {
		return false
	}
	hasCommand := false
	for _, child := range node.Children {
		if child.Type != kong.CommandNode {
			return false
		}
		hasCommand = true
	}
	return hasCommand
}

func findChildCommand(node *kong.Node, name string) *kong.Node {
	for _, child := range node.Children {
		if child.Type != kong.CommandNode {
			continue
		}
		if child.Name == name {
			return child
		}
		for _, alias := range child.Aliases {
			if alias == name {
				return child
			}
		}
	}
	return nil
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

// flagScope tracks which flags are valid at the current command path and
// whether they consume the next token as their value.
type flagScope struct {
	long  map[string]*kong.Flag
	short map[rune]*kong.Flag
}

func (s *flagScope) add(node *kong.Node) {
	if s.long == nil {
		s.long = map[string]*kong.Flag{}
		s.short = map[rune]*kong.Flag{}
	}
	for _, flag := range node.Flags {
		s.long[flag.Name] = flag
		for _, alias := range flag.Aliases {
			s.long[alias] = flag
		}
		if flag.Short != 0 {
			s.short[flag.Short] = flag
		}
	}
}

// takesValue reports whether arg is a flag whose value is the next token.
func (s *flagScope) takesValue(arg string) bool {
	var flag *kong.Flag
	switch {
	case strings.HasPrefix(arg, "--"):
		name := strings.TrimPrefix(arg, "--")
		if strings.Contains(name, "=") {
			return false
		}
		flag = s.long[name]
	case len(arg) == 2:
		flag = s.short[rune(arg[1])]
	default:
		return false // -xVALUE or bundled shorts carry their value inline
	}
	return flag != nil && !flag.IsBool() && !flag.IsCounter()
}
