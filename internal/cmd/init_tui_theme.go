package cmd

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/nimbu/cli/internal/output"
)

// initTheme is the init wizard's palette as semantic tokens, taken from the
// CLI's shared palette (output.PaletteFor). Every color the wizard draws comes
// from here. Color is never the only signal: markers (◇ ● › ○), labels and
// copy carry the meaning on their own, so the wizard reads the same with color
// off.
type initTheme struct {
	enabled bool

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
		return initTheme{}
	}
	palette := output.PaletteFor(isDark)
	return initTheme{
		enabled: true,
		Done:    lipgloss.Color(palette.Success),
		Active:  lipgloss.Color(palette.Accent),
		Muted:   lipgloss.Color(palette.Muted),
		Border:  lipgloss.Color(palette.Border),
		Warn:    lipgloss.Color(palette.Warning),
	}
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
