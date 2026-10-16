package cmd

import (
	"context"
	"math"
	"os"
	"slices"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"
	"golang.org/x/term"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// BannerTheme holds a named color palette for the server banner.
type BannerTheme struct {
	Name  string
	Label string
	// Palette has one hex color per banner line, tuned for dark terminal
	// backgrounds. It is also the fallback when the background is unknown.
	// An empty entry means "the terminal's own text color".
	Palette []string
	// Light is the same palette tuned for light terminal backgrounds.
	Light []string
}

// PaletteFor returns the palette variant for a dark or light background.
func (t BannerTheme) PaletteFor(dark bool) []string {
	if dark || len(t.Light) == 0 {
		return t.Palette
	}
	return t.Light
}

// bannerMinContrast is the minimum WCAG contrast ratio every banner color
// keeps against a typical background. 3:1 is the WCAG threshold for large,
// bold text, which the block-letter banner is.
const bannerMinContrast = 3.0

// Reference backgrounds: a common dark theme gray and a common light one.
var (
	bannerDarkBackground  = bannerHex("#1e1e1e")
	bannerLightBackground = bannerHex("#fafafa")
)

type bannerThemeSpec struct {
	name, label string
	colors      []string
}

// bannerThemeSpecs are the design palettes. buildBannerThemes derives a dark
// and a light variant from each so every line stays readable on both.
var bannerThemeSpecs = []bannerThemeSpec{
	// The default uses the terminal's own text color, which is readable on any
	// background without having to detect it.
	{"default", "Default (terminal text color)", []string{"", "", "", "", "", ""}},
	{"rainbow", "Rainbow", []string{"#ff0000", "#ff8800", "#ffff00", "#00cc00", "#0066ff", "#8800ff"}},
	{"gradient", "Gradient", []string{"#38bdf8", "#22d3ee", "#34d399", "#facc15", "#fb7185", "#ef4444"}},
	{"ocean", "Ocean", []string{"#1e3a5f", "#2563eb", "#3b82f6", "#60a5fa", "#93c5fd", "#bfdbfe"}},
	{"sunset", "Sunset", []string{"#7c2d12", "#c2410c", "#ea580c", "#f97316", "#fb923c", "#fdba74"}},
	{"forest", "Forest", []string{"#14532d", "#166534", "#15803d", "#22c55e", "#4ade80", "#86efac"}},
	{"neon", "Neon", []string{"#f0abfc", "#c084fc", "#a78bfa", "#818cf8", "#6366f1", "#4f46e5"}},
	{"candy", "Candy", []string{"#fca5a5", "#fdba74", "#fde68a", "#86efac", "#93c5fd", "#d8b4fe"}},
	{"cyberpunk", "Cyberpunk", []string{"#ff003c", "#ff2a6d", "#d300c5", "#7b00ff", "#3c1361", "#1a1a2e"}},
	{"aurora", "Aurora", []string{"#00ff87", "#00e4a0", "#00c8b8", "#00acd1", "#6366f1", "#a855f7"}},
	{"lava", "Lava", []string{"#fff7ed", "#fed7aa", "#fdba74", "#f97316", "#ea580c", "#9a3412"}},
	{"ice", "Ice", []string{"#f0f9ff", "#bae6fd", "#7dd3fc", "#38bdf8", "#0ea5e9", "#0369a1"}},
	{"dracula", "Dracula", []string{"#ff79c6", "#bd93f9", "#8be9fd", "#50fa7b", "#f1fa8c", "#ffb86c"}},
	{"catppuccin", "Catppuccin", []string{"#f38ba8", "#fab387", "#f9e2af", "#a6e3a1", "#89b4fa", "#cba6f7"}},
	{"synthwave", "Synthwave", []string{"#f6d365", "#ff6a88", "#ff2a6d", "#c31dde", "#7b2fef", "#241734"}},
	{"nord", "Nord", []string{"#88c0d0", "#81a1c1", "#5e81ac", "#b48ead", "#a3be8c", "#ebcb8b"}},
	{"matrix", "Matrix", []string{"#0d3b0d", "#14691a", "#1fa12a", "#39e645", "#7cfc7c", "#c4ffc4"}},
	{"sakura", "Sakura", []string{"#fdf2f8", "#fce7f3", "#fbcfe8", "#f9a8d4", "#f472b6", "#ec4899"}},
}

// bannerThemes derives the palettes on first use rather than at package init:
// the contrast fitting takes a couple of milliseconds, which every command
// (including shell completion) would otherwise pay at startup.
var bannerThemes = sync.OnceValue(func() []BannerTheme {
	return buildBannerThemes(bannerThemeSpecs)
})

func buildBannerThemes(specs []bannerThemeSpec) []BannerTheme {
	themes := make([]BannerTheme, len(specs))
	for i, spec := range specs {
		themes[i] = BannerTheme{
			Name:    spec.name,
			Label:   spec.label,
			Palette: fitPaletteToBackground(spec.colors, true),
			Light:   fitPaletteToBackground(spec.colors, false),
		}
	}
	return themes
}

// BannerThemes returns the ordered list of available themes.
func BannerThemes() []BannerTheme {
	return bannerThemes()
}

// BannerThemeByName looks up a theme by name (case-insensitive).
func BannerThemeByName(name string) (BannerTheme, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, t := range bannerThemes() {
		if t.Name == lower {
			return t, true
		}
	}
	return BannerTheme{}, false
}

// BannerThemeNames returns the names of all available themes.
func BannerThemeNames() []string {
	names := make([]string, len(bannerThemeSpecs))
	for i, spec := range bannerThemeSpecs {
		names[i] = spec.name
	}
	return names
}

// DefaultBannerPalette returns the default palette. It renders in the
// terminal's own text color, so it is readable on dark and light backgrounds
// alike without detecting which one is in use.
func DefaultBannerPalette() []string {
	return bannerThemes()[0].Palette
}

// ShouldShowBanner is the one rule for decorative banner art (nimbu server,
// nimbu init, both the line path and the wizard): output is for humans (no
// --json/--plain), stderr is a terminal, and the process is not in CI (CI set
// to anything but 0/false). The banner always draws on stderr.
func ShouldShowBanner(w *output.Writer) bool {
	if w == nil || w.Mode.JSON || w.Mode.Plain {
		return false
	}
	return shouldShowBanner(bannerStderrIsTerminal(w), ciEnvironment())
}

// bannerStderrIsTerminal reports whether stderr is a terminal; swapped in
// tests, which write to buffers.
var bannerStderrIsTerminal = func(w *output.Writer) bool { return w.ErrIsTTY() }

func shouldShowBanner(stderrTTY, ci bool) bool {
	return stderrTTY && !ci
}

// configuredBannerTheme returns the banner theme named in the user config,
// or "" for the default.
func configuredBannerTheme(ctx context.Context) string {
	if cfg, ok := ctx.Value(configKey{}).(*config.Config); ok && cfg != nil {
		return cfg.BannerTheme
	}
	return ""
}

// bannerThemeOrDefault looks a theme up by name, falling back to the default.
func bannerThemeOrDefault(name string) BannerTheme {
	if theme, ok := BannerThemeByName(name); ok {
		return theme
	}
	return bannerThemes()[0]
}

// ResolveBannerPalette returns the palette for the named theme (the default
// when the name is empty or unknown), picking the light or dark variant for
// the terminal stderr draws on.
//
// The background is queried at most once per process, and only when stderr
// gets color and both stdin and stderr are terminals; otherwise the dark
// variant is used. Do not call it while a Bubble Tea program runs (the query
// would compete with it for input): resolve before starting the program, or
// inside one use tea.RequestBackgroundColor and BannerTheme.PaletteFor.
func ResolveBannerPalette(w *output.Writer, name string) []string {
	return bannerThemeOrDefault(name).PaletteFor(lookupBannerBackground(w))
}

// lookupBannerBackground is the background lookup line paths use; swapped in
// tests, which have no terminal to ask.
var lookupBannerBackground = bannerBackgroundIsDark

var (
	bannerBackgroundOnce sync.Once
	bannerBackgroundDark = true
	// queryDarkBackground asks the terminal for its background; swapped in tests.
	queryDarkBackground = func(in, out *os.File) bool { return lipgloss.HasDarkBackground(in, out) }
)

// bannerBackgroundIsDark reports whether stderr's terminal has a dark
// background, querying it at most once per process and only where the answer
// matters and can arrive (see ResolveBannerPalette). Line output that draws
// next to the banner uses it to pick the matching chrome palette.
func bannerBackgroundIsDark(w *output.Writer) bool {
	if w == nil || !w.ErrUseColor() || !w.ErrIsTTY() || !term.IsTerminal(int(os.Stdin.Fd())) {
		return true
	}
	errFile, ok := w.Err.(*os.File)
	if !ok {
		return true
	}
	bannerBackgroundOnce.Do(func() {
		bannerBackgroundDark = queryDarkBackground(os.Stdin, errFile)
	})
	return bannerBackgroundDark
}

// fitPaletteToBackground compresses a palette's lightness just enough that
// every color reaches bannerMinContrast against the reference background,
// keeping hue and the top-to-bottom gradient order. For a dark background the
// lightness range is lifted toward white; for a light one it is scaled down
// toward black. A palette that is already readable is returned unchanged, and
// empty entries (terminal text color) pass through.
func fitPaletteToBackground(palette []string, dark bool) []string {
	type hcl struct{ h, c, l float64 }
	parsed := make([]*hcl, len(palette))
	for i, hex := range palette {
		if hex == "" {
			continue
		}
		h, c, l := bannerHex(hex).Hcl()
		parsed[i] = &hcl{h, c, l}
	}

	// render maps every color with compression t (0 = none, 1 = full) and
	// reports whether all of them reach the minimum contrast.
	render := func(t float64) ([]string, bool) {
		out := make([]string, len(palette))
		ok := true
		for i, p := range parsed {
			if p == nil {
				continue
			}
			l := p.l * (1 - t) // light background: cap lightness at 1-t
			if dark {
				l = t + p.l*(1-t) // dark background: floor lightness at t
			}
			col := colorful.Hcl(p.h, p.c, l).Clamped()
			ok = ok && meetsBannerContrast(col, dark)
			out[i] = col.Hex()
		}
		return out, ok
	}

	if _, ok := render(0); ok {
		return slices.Clone(palette)
	}
	for step := 1; step < 100; step++ {
		if out, ok := render(float64(step) / 100); ok {
			return out
		}
	}
	out, _ := render(1)
	return out
}

func meetsBannerContrast(col colorful.Color, dark bool) bool {
	bg := bannerLightBackground
	if dark {
		bg = bannerDarkBackground
	}
	return bannerContrastRatio(col, bg) >= bannerMinContrast
}

// bannerContrastRatio is the WCAG 2 contrast ratio between two colors.
func bannerContrastRatio(a, b colorful.Color) float64 {
	la, lb := bannerLuminance(a), bannerLuminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

func bannerLuminance(c colorful.Color) float64 {
	r, g, b := c.LinearRgb()
	return 0.2126*r + 0.7152*g + 0.0722*b
}

func bannerHex(hex string) colorful.Color {
	col, err := colorful.Hex(hex)
	if err != nil {
		panic("banner theme: invalid color " + hex)
	}
	return col
}
