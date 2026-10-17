package cmd

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/nimbu/cli/internal/output"
)

// helpOptions returns Kong help configuration options.
func helpOptions() kong.HelpOptions {
	return kong.HelpOptions{
		NoExpandSubcommands: true,
	}
}

// Help width bounds: prose wider than maxHelpWidth is hard to read.
const (
	defaultHelpWidth = 80
	maxHelpWidth     = 100
)

// helpPrinter returns the HelpPrinter that renders nimbu's help from the
// Kong model (sections, safety flags, examples) and colors it for stdout.
func helpPrinter() kong.HelpPrinter {
	return func(options kong.HelpOptions, ctx *kong.Context) error {
		profile := helpColorProfile(ctx.Args, ctx.Stdout)
		text := renderHelp(ctx, helpRenderOptions{
			Summary: options.Summary && !ctx.Empty(),
			ShowAll: helpAllRequested(ctx.Args),
			Width:   helpWidth(ctx.Stdout),
			Style:   newHelpStyle(profile),
		})
		_, err := io.WriteString(ctx.Stdout, text)
		return err
	}
}

// helpAllRequested reports whether --help-all was passed. Help is printed
// while Kong is still parsing, so the raw args are the only source.
func helpAllRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--help-all" {
			return true
		}
	}
	return false
}

// helpColorMode returns the --color mode for help output. Help is printed
// before flags are applied, so --color is read from the raw args, then
// NIMBU_COLOR. Machine output (--json/--plain or NIMBU_JSON/NIMBU_PLAIN) turns
// "auto" into "never": only an explicit always/never overrides it. --no-input
// is not a color signal (see newRootWriter); color stays per stream.
func helpColorMode(args []string) string {
	mode := helpColorArg(args)
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(os.Getenv("NIMBU_COLOR")))
	}
	if mode == "" {
		mode = "auto"
	}
	if mode == "auto" && helpMachineOutput(args) {
		return "never"
	}
	return mode
}

// helpColorArg returns the --color value from the raw args, or "".
func helpColorArg(args []string) string {
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--color" && i+1 < len(args) {
			return strings.ToLower(strings.TrimSpace(args[i+1]))
		}
		if value, ok := strings.CutPrefix(arg, "--color="); ok {
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	return ""
}

// helpMachineOutput reports whether --json/--plain (or their env vars) ask
// for machine output. As in the parser, a flag in the args beats its env var
// (--json=false turns NIMBU_JSON=1 off) and the last occurrence wins.
func helpMachineOutput(args []string) bool {
	json, jsonSet := helpBoolFlag(args, "--json")
	if !jsonSet {
		json = envTruthy(os.Getenv("NIMBU_JSON"))
	}
	plain, plainSet := helpBoolFlag(args, "--plain")
	if !plainSet {
		plain = envTruthy(os.Getenv("NIMBU_PLAIN"))
	}
	return json || plain
}

// helpBoolFlag returns the value of a boolean flag in the raw args (bare or
// --flag=value, last one wins) and whether it was given at all. A value kong
// would reject is ignored.
func helpBoolFlag(args []string, flag string) (value, set bool) {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == flag {
			value, set = true, true
			continue
		}
		if v, ok := strings.CutPrefix(arg, flag+"="); ok {
			if on, valid := kongBool(v); valid {
				value, set = on, true
			}
		}
	}
	return value, set
}

// helpColorProfile decides color for help through the shared output color
// API: help is written to stdout, so the stdout TTY decides "auto".
func helpColorProfile(args []string, stdout io.Writer) termenv.Profile {
	w := &output.Writer{Out: stdout, Err: os.Stderr, Color: helpColorMode(args)}
	return w.OutColorProfile()
}

// helpWidth is the wrap width: $COLUMNS, else the terminal width, else 80,
// capped at maxHelpWidth.
func helpWidth(w io.Writer) int {
	width := 0
	if cols, err := strconv.Atoi(strings.TrimSpace(os.Getenv("COLUMNS"))); err == nil && cols > 0 {
		width = cols
	} else if f, ok := w.(*os.File); ok {
		if cols, _, err := term.GetSize(int(f.Fd())); err == nil && cols > 0 {
			width = cols
		}
	}
	if width <= 0 {
		width = defaultHelpWidth
	}
	if width > maxHelpWidth {
		width = maxHelpWidth
	}
	return width
}

// helpStyle colors the parts of rendered help. The plain style returns text
// unchanged, so layout is computed on plain text and styled afterwards.
type helpStyle struct {
	usage   func(string) string
	section func(string) string
	command func(string) string
	flag    func(string) string
	comment func(string) string
}

func plainHelpStyle() helpStyle {
	same := func(s string) string { return s }
	return helpStyle{usage: same, section: same, command: same, flag: same, comment: same}
}

// newHelpStyle colors help with the shared semantic theme: headings in the
// accent color, command names like paths, flags and comments muted.
// Descriptions keep the terminal's own foreground.
func newHelpStyle(profile termenv.Profile) helpStyle {
	theme := output.NewTheme(profile)
	if !theme.Enabled() {
		return plainHelpStyle()
	}
	palette := theme.Palette()
	color := func(hex string, bold bool) func(string) string {
		return func(s string) string { return theme.Paint(s, hex, bold) }
	}
	return helpStyle{
		usage:   color(palette.Accent, true),
		section: color(palette.Accent, true),
		command: color(palette.Path, true),
		flag:    color(palette.Muted, false),
		comment: color(palette.Muted, false),
	}
}
