package cmd

import (
	"bytes"
	"os"
	"slices"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

// Regression: the default palette was pure white, invisible on light
// terminals, and several themes faded into a dark background.
func TestBannerThemesReadableOnBothBackgrounds(t *testing.T) {
	for _, theme := range BannerThemes() {
		for _, variant := range []struct {
			name    string
			dark    bool
			palette []string
		}{{"dark", true, theme.Palette}, {"light", false, theme.Light}} {
			if len(variant.palette) != 6 {
				t.Fatalf("%s/%s has %d colors, want one per banner line", theme.Name, variant.name, len(variant.palette))
			}
			for i, hex := range variant.palette {
				if hex == "" {
					continue // terminal text color
				}
				if !meetsBannerContrast(bannerHex(hex), variant.dark) {
					t.Errorf("%s/%s line %d %s is below %.1f:1 contrast", theme.Name, variant.name, i, hex, bannerMinContrast)
				}
			}
		}
	}
}

func TestDefaultBannerPaletteUsesTerminalTextColor(t *testing.T) {
	theme, ok := BannerThemeByName("default")
	if !ok {
		t.Fatal("default theme missing")
	}
	for _, palette := range [][]string{DefaultBannerPalette(), theme.PaletteFor(true), theme.PaletteFor(false)} {
		for i, hex := range palette {
			if hex != "" {
				t.Fatalf("default line %d = %q, want the terminal text color", i, hex)
			}
		}
	}
}

func TestFitPaletteKeepsReadablePalettesUnchanged(t *testing.T) {
	gradient := []string{"#38bdf8", "#22d3ee", "#34d399", "#facc15", "#fb7185", "#ef4444"}
	if got := fitPaletteToBackground(gradient, true); !slices.Equal(got, gradient) {
		t.Fatalf("readable dark palette changed: %v", got)
	}
}

func TestFitPaletteKeepsGradientOrder(t *testing.T) {
	ice := []string{"#f0f9ff", "#bae6fd", "#7dd3fc", "#38bdf8", "#0ea5e9", "#0369a1"}
	light := fitPaletteToBackground(ice, false)
	for i := 1; i < len(light); i++ {
		_, _, prev := bannerHex(light[i-1]).Hcl()
		_, _, cur := bannerHex(light[i]).Hcl()
		if cur > prev+1e-9 {
			t.Fatalf("light ice palette lost its light-to-dark order at %d: %v", i, light)
		}
	}
}

func TestBannerThemePaletteFor(t *testing.T) {
	theme, _ := BannerThemeByName("sakura")
	if !slices.Equal(theme.PaletteFor(true), theme.Palette) || !slices.Equal(theme.PaletteFor(false), theme.Light) {
		t.Fatal("PaletteFor picks the wrong variant")
	}
	if got := (BannerTheme{Palette: []string{"#fff"}}).PaletteFor(false); !slices.Equal(got, []string{"#fff"}) {
		t.Fatalf("missing light variant should fall back to Palette, got %v", got)
	}
}

// BannerThemeNames reads the specs directly so listing names (config set
// validation, error hints) never pays for building the palettes.
func TestBannerThemeNamesMatchThemes(t *testing.T) {
	themes := BannerThemes()
	names := BannerThemeNames()
	if len(names) != len(themes) {
		t.Fatalf("%d names for %d themes", len(names), len(themes))
	}
	for i, theme := range themes {
		if names[i] != theme.Name {
			t.Fatalf("name %d = %q, want %q", i, names[i], theme.Name)
		}
	}
}

func TestBannerThemeByNameIsCaseInsensitive(t *testing.T) {
	if theme, ok := BannerThemeByName("  Ocean "); !ok || theme.Name != "ocean" {
		t.Fatalf("lookup = %+v, %v", theme, ok)
	}
}

func TestShouldShowBanner(t *testing.T) {
	cases := []struct {
		tty  bool
		ci   string
		want bool
	}{
		{true, "", true},
		{false, "", false},
		{true, "true", false},
		{true, "1", false},
	}
	for _, c := range cases {
		if got := shouldShowBanner(c.tty, c.ci); got != c.want {
			t.Errorf("shouldShowBanner(%v, %q) = %v, want %v", c.tty, c.ci, got, c.want)
		}
	}
	// A piped stderr never shows the banner.
	if ShouldShowBanner(&output.Writer{Err: &bytes.Buffer{}}) {
		t.Fatal("banner shown on a non-terminal stderr")
	}
}

// The background query must never run when color is off or stderr is not a
// terminal: it writes an escape sequence and waits for the terminal to answer.
func TestResolveBannerPaletteSkipsQueryWithoutTerminal(t *testing.T) {
	orig := queryDarkBackground
	t.Cleanup(func() { queryDarkBackground = orig })
	queryDarkBackground = func(_, _ *os.File) bool {
		t.Fatal("background queried without a color terminal")
		return true
	}

	theme, _ := BannerThemeByName("ice")
	writers := []*output.Writer{
		{Err: &bytes.Buffer{}, Color: "always"},
		{Err: os.Stderr, Color: "never"},
		{Err: os.Stderr, NoTTY: true},
	}
	for _, w := range writers {
		if got := ResolveBannerPalette(w, "ice"); !slices.Equal(got, theme.Palette) {
			t.Fatalf("palette = %v, want the dark fallback", got)
		}
	}
	if got := ResolveBannerPalette(writers[0], "nope"); !slices.Equal(got, DefaultBannerPalette()) {
		t.Fatalf("unknown theme = %v, want default", got)
	}
}
