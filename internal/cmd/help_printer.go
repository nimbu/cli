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

// Color palette (matches gogcli/frontappcli). Descriptions keep the
// terminal's default foreground so help stays readable on light themes.
const (
	colorUsage   = "#60a5fa" // blue - Usage heading
	colorSection = "#a78bfa" // purple - section titles
	colorCommand = "#38bdf8" // cyan - command names
	colorDim     = "#9ca3af" // gray - flags, placeholders, example comments
)

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
// NIMBU_COLOR. --json/--plain ask for machine output, so they turn color off
// unless --color says otherwise.
func helpColorMode(args []string) string {
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
	if v := strings.TrimSpace(os.Getenv("NIMBU_COLOR")); v != "" {
		return strings.ToLower(v)
	}
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" || arg == "--plain" {
			return "never"
		}
	}
	return "auto"
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

func newHelpStyle(profile termenv.Profile) helpStyle {
	if profile == termenv.Ascii {
		return plainHelpStyle()
	}
	color := func(hex string, bold bool) func(string) string {
		return func(s string) string {
			if s == "" {
				return s
			}
			styled := profile.String(s).Foreground(profile.Color(hex))
			if bold {
				styled = styled.Bold()
			}
			return styled.String()
		}
	}
	return helpStyle{
		usage:   color(colorUsage, true),
		section: color(colorSection, true),
		command: color(colorCommand, true),
		flag:    color(colorDim, false),
		comment: color(colorDim, false),
	}
}
