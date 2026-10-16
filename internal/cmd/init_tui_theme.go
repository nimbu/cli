package cmd

import (
	"image/color"
	"slices"

	"charm.land/lipgloss/v2"
)

// initTheme is the init wizard's palette as semantic tokens. Every color the
// wizard draws comes from here. Color is never the only signal: markers (◇ ●
// › ○), labels and copy carry the meaning on their own, so the wizard reads the
// same with color off.
type initTheme struct {
	enabled bool

	Done   color.Color // finished-step markers and the "Done!" label
	Active color.Color // current-step marker, selection markers, primary action
	Muted  color.Color // labels, rails, hints
	Text   color.Color // values the user picked or typed
	Border color.Color // banner frame
	Warn   color.Color // "finishing…" notice while files are written
}

// newInitTheme builds the palette for a dark or light terminal background.
// With color off every token is nil and styles render plain text.
func newInitTheme(useColor, isDark bool) initTheme {
	if !useColor {
		return initTheme{}
	}
	pick := lipgloss.LightDark(isDark)
	return initTheme{
		enabled: true,
		Done:    pick(lipgloss.Color("#15803d"), lipgloss.Color("#22c55e")),
		Active:  pick(lipgloss.Color("#1d4ed8"), lipgloss.Color("#60a5fa")),
		Muted:   pick(lipgloss.Color("#475569"), lipgloss.Color("#94a3b8")),
		Text:    pick(lipgloss.Color("#0f172a"), lipgloss.Color("#e2e8f0")),
		Border:  pick(lipgloss.Color("#64748b"), lipgloss.Color("#64748b")),
		Warn:    pick(lipgloss.Color("#b45309"), lipgloss.Color("#f59e0b")),
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

// bannerLineColor returns the color for banner row idx. The default banner
// palette is pure white, which disappears on a light background, so it maps to
// the Text token instead; named banner themes keep their own colors.
func (t initTheme) bannerLineColor(palette []string, idx int) color.Color {
	if !t.enabled {
		return nil
	}
	if len(palette) == 0 || slices.Equal(palette, DefaultBannerPalette()) {
		return t.Text
	}
	return lipgloss.Color(palette[idx%len(palette)])
}
