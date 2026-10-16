package output

import (
	"io"
	"os"
	"strings"

	"github.com/muesli/termenv"
)

// Semantic color tokens. Hex values are degraded by termenv to the nearest
// 256/16-color entry on less capable terminals.
const (
	colorSuccess = "#22c55e" // green
	colorWarning = "#f59e0b" // amber, also used for dry-run accents
	colorError   = "#ef4444" // red
	colorAccent  = "#60a5fa" // blue: spinners, active work
	colorMuted   = "#94a3b8" // slate: rails, secondary text
	colorPath    = "#38bdf8" // sky: file paths, identifiers
)

// Theme styles text with the CLI's semantic palette. The zero value (or a
// Theme built with color off) returns text unchanged, so callers never need
// their own "if color" branches.
type Theme struct {
	out *termenv.Output
}

// NewTheme builds a theme for a termenv profile; termenv.Ascii disables color.
func NewTheme(profile termenv.Profile) Theme {
	if profile == termenv.Ascii {
		return Theme{}
	}
	return Theme{out: termenv.NewOutput(io.Discard, termenv.WithProfile(profile))}
}

// OutTheme returns the theme for content written to stdout.
func (w *Writer) OutTheme() Theme { return NewTheme(w.OutColorProfile()) }

// ErrTheme returns the theme for chrome written to stderr.
func (w *Writer) ErrTheme() Theme { return NewTheme(w.ErrColorProfile()) }

// Enabled reports whether the theme emits escape sequences.
func (t Theme) Enabled() bool { return t.out != nil }

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
func (t Theme) Success(s string) string { return t.style(s, colorSuccess, true) }

// Warning marks warnings and dry-run accents (bold amber).
func (t Theme) Warning(s string) string { return t.style(s, colorWarning, true) }

// WarningText colors longer warning prose without bold.
func (t Theme) WarningText(s string) string { return t.style(s, colorWarning, false) }

// Error marks failures (bold red).
func (t Theme) Error(s string) string { return t.style(s, colorError, true) }

// Accent marks active work such as spinners (bold blue).
func (t Theme) Accent(s string) string { return t.style(s, colorAccent, true) }

// Muted de-emphasizes secondary text such as rails, hints and footers.
func (t Theme) Muted(s string) string { return t.style(s, colorMuted, false) }

// Path highlights file paths and identifiers.
func (t Theme) Path(s string) string { return t.style(s, colorPath, false) }

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
