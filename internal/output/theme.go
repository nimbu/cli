package output

import (
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/muesli/termenv"
)

// Palette is the CLI's one semantic chrome palette, as hex colors. Every
// non-banner color (line output, the init wizard, the banner picker, help,
// nimbu server) comes from here, so meaning and tone stay consistent.
//
// There is deliberately no "text" token: body text and values keep the
// terminal's own foreground (use Strong for emphasis), which is readable on
// any background. Meaning never rests on color alone; markers and words carry
// it too.
type Palette struct {
	Success string // completed work, healthy state
	Warning string // warnings, dry-run accents, unverified state
	Error   string // failures
	Accent  string // active work, selections, primary actions, headings
	Muted   string // rails, labels, hints, secondary text
	Path    string // file paths, identifiers, types
	Border  string // frames and separators (non-text)
}

// darkPalette is tuned for dark terminal backgrounds. It is also the default
// when the background is unknown: line output never queries the terminal.
var darkPalette = Palette{
	Success: "#22c55e",
	Warning: "#f59e0b",
	Error:   "#ef4444",
	Accent:  "#60a5fa",
	Muted:   "#94a3b8",
	Path:    "#38bdf8",
	Border:  "#64748b",
}

// lightPalette holds the same tokens, darkened for light backgrounds.
var lightPalette = Palette{
	Success: "#15803d",
	Warning: "#b45309",
	Error:   "#b91c1c",
	Accent:  "#1d4ed8",
	Muted:   "#475569",
	Path:    "#0369a1",
	Border:  "#64748b",
}

// PaletteFor returns the palette variant for a dark or light background.
func PaletteFor(dark bool) Palette {
	if dark {
		return darkPalette
	}
	return lightPalette
}

// CharmProfile maps a termenv profile (the output package's per-stream color
// decision) to the colorprofile that Bubble Tea and Lipgloss v2 render with.
func CharmProfile(profile termenv.Profile) colorprofile.Profile {
	switch profile {
	case termenv.TrueColor:
		return colorprofile.TrueColor
	case termenv.ANSI256:
		return colorprofile.ANSI256
	case termenv.ANSI:
		return colorprofile.ANSI
	default:
		return colorprofile.ASCII
	}
}

// Theme styles text with the CLI's semantic palette, downsampled to the
// stream's color profile. The zero value (or a Theme built with color off)
// returns text unchanged, so callers never need their own "if color" branches.
type Theme struct {
	out     *termenv.Output
	palette Palette
}

// NewTheme builds a theme for a termenv profile with the default (dark)
// palette; termenv.Ascii disables color.
func NewTheme(profile termenv.Profile) Theme {
	return NewThemeFor(profile, true)
}

// NewThemeFor builds a theme for a profile and a known terminal background.
func NewThemeFor(profile termenv.Profile, dark bool) Theme {
	if profile == termenv.Ascii {
		return Theme{}
	}
	return Theme{
		out:     termenv.NewOutput(io.Discard, termenv.WithProfile(profile)),
		palette: PaletteFor(dark),
	}
}

// OutTheme returns the theme for content written to stdout.
func (w *Writer) OutTheme() Theme { return NewTheme(w.OutColorProfile()) }

// ErrTheme returns the theme for chrome written to stderr.
func (w *Writer) ErrTheme() Theme { return NewTheme(w.ErrColorProfile()) }

// Enabled reports whether the theme emits escape sequences.
func (t Theme) Enabled() bool { return t.out != nil }

// Palette returns the theme's palette (the dark one for a disabled theme).
func (t Theme) Palette() Palette {
	if t.out == nil {
		return darkPalette
	}
	return t.palette
}

// Paint styles s with a hex color (empty keeps the terminal's foreground),
// optionally bold, downsampled to the theme's profile.
func (t Theme) Paint(s, hex string, bold bool) string {
	return t.style(s, hex, bold)
}

func (t Theme) style(s, hex string, bold bool) string {
	if t.out == nil || s == "" {
		return s
	}
	styled := t.out.String(s)
	if hex != "" {
		styled = styled.Foreground(t.out.Color(hex))
	}
	if bold {
		styled = styled.Bold()
	}
	return styled.String()
}

// Success marks completed work (bold green).
func (t Theme) Success(s string) string { return t.style(s, t.palette.Success, true) }

// Warning marks warnings and dry-run accents (bold amber).
func (t Theme) Warning(s string) string { return t.style(s, t.palette.Warning, true) }

// WarningText colors longer warning prose without bold.
func (t Theme) WarningText(s string) string { return t.style(s, t.palette.Warning, false) }

// Error marks failures (bold red).
func (t Theme) Error(s string) string { return t.style(s, t.palette.Error, true) }

// Accent marks active work such as spinners (bold blue).
func (t Theme) Accent(s string) string { return t.style(s, t.palette.Accent, true) }

// Muted de-emphasizes secondary text such as rails, hints and footers.
func (t Theme) Muted(s string) string { return t.style(s, t.palette.Muted, false) }

// Path highlights file paths and identifiers.
func (t Theme) Path(s string) string { return t.style(s, t.palette.Path, false) }

// Border colors frames and separators.
func (t Theme) Border(s string) string { return t.style(s, t.palette.Border, false) }

// Strong emphasizes headings and names. It only adds bold and keeps the
// terminal's own foreground, so it stays readable on light and dark themes.
func (t Theme) Strong(s string) string { return t.style(s, "", true) }

// Faint renders text with the terminal's faint attribute.
func (t Theme) Faint(s string) string {
	if t.out == nil || s == "" {
		return s
	}
	return t.out.String(s).Faint().String()
}

// Glyphs is the symbol set used for status markers and timeline chrome.
// Every glyph has an ASCII fallback, and status glyphs are always paired with
// a word so meaning survives without color or Unicode.
type Glyphs struct {
	OK        string // success marker
	Fail      string // failure marker
	Warn      string // warning marker
	Arrow     string // from -> to
	Step      string // completed timeline stage
	SubStep   string // completed sub-item inside a stage
	Pending   string // planned (dry-run) item
	Top       string // timeline opening corner
	Rail      string // timeline connector
	Bottom    string // timeline closing corner
	Dash      string // separator between a name and its summary
	Ellipsis  string // truncation marker
	Spinner   []string
	ASCIIOnly bool
}

var unicodeGlyphs = Glyphs{
	OK:       "✓",
	Fail:     "✗",
	Warn:     "⚠",
	Arrow:    "→",
	Step:     "◇",
	SubStep:  "◦",
	Pending:  "○",
	Top:      "┌",
	Rail:     "│",
	Bottom:   "└",
	Dash:     "—",
	Ellipsis: "…",
	Spinner:  []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
}

var asciiGlyphs = Glyphs{
	OK:        "ok",
	Fail:      "x",
	Warn:      "!",
	Arrow:     "->",
	Step:      "*",
	SubStep:   "-",
	Pending:   "o",
	Top:       "+",
	Rail:      "|",
	Bottom:    "+",
	Dash:      "-",
	Ellipsis:  "...",
	Spinner:   []string{"-", "\\", "|", "/"},
	ASCIIOnly: true,
}

// GlyphSet returns the Unicode or ASCII glyph set.
func GlyphSet(ascii bool) Glyphs {
	if ascii {
		return asciiGlyphs
	}
	return unicodeGlyphs
}

// Glyphs returns the glyph set for this environment.
func (w *Writer) Glyphs() Glyphs { return GlyphSet(ASCIIOnly()) }

// ASCIIOnly reports whether output should avoid non-ASCII glyphs: TERM=dumb,
// NIMBU_ASCII=1, or a locale that is set but not UTF-8 (LC_ALL, then
// LC_CTYPE, then LANG). An unset locale keeps Unicode, since most terminals
// that run without one (containers, CI) still render UTF-8.
func ASCIIOnly() bool { return asciiOnly(os.Getenv) }

func asciiOnly(getenv func(string) string) bool {
	if strings.EqualFold(getenv("TERM"), "dumb") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(getenv("NIMBU_ASCII"))) {
	case "1", "true", "yes":
		return true
	}
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		value := strings.TrimSpace(getenv(key))
		if value == "" {
			continue
		}
		lower := strings.ToLower(value)
		return !strings.Contains(lower, "utf-8") && !strings.Contains(lower, "utf8")
	}
	return false
}

// ErrAnimated reports whether stderr may carry live, cursor-moving chrome
// (spinners, redrawn lines): stderr must be a terminal, progress must not be
// disabled, and TERM must not be "dumb".
func (w *Writer) ErrAnimated() bool {
	return w != nil && animationAllowed(w.NoSpin, w.ErrIsTTY(), os.Getenv)
}

func animationAllowed(noSpin, isTTY bool, getenv func(string) string) bool {
	if noSpin || !isTTY {
		return false
	}
	return !strings.EqualFold(getenv("TERM"), "dumb")
}
