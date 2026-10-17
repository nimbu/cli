package cmd

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nimbu/cli/internal/output"
)

// initTheme is the init wizard's palette as semantic tokens, taken from the
// CLI's shared palette (output.PaletteFor). Every color the wizard draws comes
// from here. Color is never the only signal: markers (◇ ● › ○), labels and
// copy carry the meaning on their own, so the wizard reads the same with color
// off.
type initTheme struct {
	enabled bool
	glyphs  initGlyphs

	Done   color.Color // finished-step markers and the "Done!" label
	Active color.Color // current-step marker, selection markers, primary action
	Muted  color.Color // labels, rails, hints
	Text   color.Color // values the user picked or typed: nil keeps the terminal's own foreground
	Border color.Color // banner frame
	Warn   color.Color // "finishing…" notice while files are written
}

// newInitTheme builds the palette for a dark or light terminal background.
// With color off every token is nil and styles render plain text.
func newInitTheme(useColor, isDark bool) initTheme {
	if !useColor {
		return initTheme{glyphs: initUnicodeGlyphs}
	}
	palette := output.PaletteFor(isDark)
	return initTheme{
		enabled: true,
		glyphs:  initUnicodeGlyphs,
		Done:    lipgloss.Color(palette.Success),
		Active:  lipgloss.Color(palette.Accent),
		Muted:   lipgloss.Color(palette.Muted),
		Border:  lipgloss.Color(palette.Border),
		Warn:    lipgloss.Color(palette.Warning),
	}
}

// initGlyphs is the wizard's symbol set. ASCII-only output (NIMBU_ASCII,
// TERM=dumb or a non-UTF-8 locale, see output.ASCIIOnly) swaps every one.
type initGlyphs struct {
	Top, Rail, Bottom string // timeline corner, connector and closing corner
	Done, Active      string // finished and current step markers
	Cursor            string // list cursor
	On, Off           string // multi-select checked and unchecked
	Up, Down          string // "N more" indicators
	Move              string // move-key hint
	Sep               string // hint separator
	Times             string // "80×24"
	Ellipsis          string // truncation marker
	Spinner           []string
}

var initUnicodeGlyphs = initGlyphs{
	Top:      "┌",
	Rail:     "│",
	Bottom:   "└",
	Done:     "◇",
	Active:   "●",
	Cursor:   "›",
	On:       "●",
	Off:      "○",
	Up:       "↑",
	Down:     "↓",
	Move:     "↑↓",
	Sep:      " · ",
	Times:    "×",
	Ellipsis: "…",
}

var initASCIIGlyphs = initGlyphs{
	Top:      "+",
	Rail:     "|",
	Bottom:   "+",
	Done:     "*",
	Active:   ">",
	Cursor:   ">",
	On:       "[x]",
	Off:      "[ ]",
	Up:       "^",
	Down:     "v",
	Move:     "up/down",
	Sep:      " - ",
	Times:    "x",
	Ellipsis: "...",
	Spinner:  output.GlyphSet(true).Spinner,
}

// truncate cuts s to width terminal cells with the theme's ellipsis, keeping
// ANSI sequences intact and counting wide (CJK, emoji) graphemes correctly.
func (t initTheme) truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	tail := t.glyphs.Ellipsis
	if tail == "" {
		tail = initUnicodeGlyphs.Ellipsis
	}
	return ansi.Truncate(s, width, tail)
}

func (t initTheme) style(fg color.Color, bold bool) lipgloss.Style {
	style := lipgloss.NewStyle()
	if !t.enabled {
		return style
	}
	if fg != nil {
		style = style.Foreground(fg)
	}
	return style.Bold(bold)
}

func (t initTheme) muted(s string) string  { return t.style(t.Muted, false).Render(s) }
func (t initTheme) value(s string) string  { return t.style(t.Text, true).Render(s) }
func (t initTheme) active(s string) string { return t.style(t.Active, true).Render(s) }
func (t initTheme) done(s string) string   { return t.style(t.Done, true).Render(s) }
func (t initTheme) warn(s string) string   { return t.style(t.Warn, true).Render(s) }

// bannerLineColor returns the color for banner row idx of a palette already
// resolved for the background (BannerTheme.PaletteFor). An empty entry, as in
// the default theme, maps to the Text token: the terminal's own foreground.
func (t initTheme) bannerLineColor(palette []string, idx int) color.Color {
	if !t.enabled || len(palette) == 0 {
		return nil
	}
	hex := palette[idx%len(palette)]
	if hex == "" {
		return t.Text
	}
	return lipgloss.Color(hex)
}

// initSpinner is the Dot spinner without its trailing padding, so the
// spinner takes exactly the marker's single cell on the timeline rail. ASCII
// output spins through "- \\ | /" instead of braille.
func initSpinner(ascii bool) spinner.Spinner {
	if ascii {
		return spinner.Spinner{Frames: initASCIIGlyphs.Spinner, FPS: spinner.Line.FPS}
	}
	frames := make([]string, 0, len(spinner.Dot.Frames))
	for _, frame := range spinner.Dot.Frames {
		frames = append(frames, strings.TrimSpace(frame))
	}
	return spinner.Spinner{Frames: frames, FPS: spinner.Dot.FPS}
}

// applyTheme rebuilds every style from the current color decision and
// background, including the embedded bubbles.
func (m *initTeaModel) applyTheme() {
	m.theme = newInitTheme(m.useColor, m.isDark)
	if m.ascii {
		m.theme.glyphs = initASCIIGlyphs
	}
	m.spinner.Style = m.theme.style(m.theme.Active, true)

	styles := textinput.DefaultStyles(m.isDark)
	if !m.useColor {
		plain := lipgloss.NewStyle()
		styles.Focused = textinput.StyleState{Text: plain, Placeholder: plain, Suggestion: plain, Prompt: plain}
		styles.Blurred = styles.Focused
	}
	// The bubbles default caret is reverse video on ANSI 7, a light grey
	// block that all but vanishes on a light background. Without a color the
	// caret reverses the terminal's own foreground, which always contrasts.
	styles.Cursor.Color = nil
	styles.Cursor.Blink = false
	m.filterInput.SetStyles(styles)
	m.directoryInput.SetStyles(styles)
}
