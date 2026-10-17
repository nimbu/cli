package output

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/termenv"
)

func envMap(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestASCIIOnly(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "unset locale keeps unicode", env: map[string]string{}, want: false},
		{name: "utf-8 lang", env: map[string]string{"LANG": "en_US.UTF-8"}, want: false},
		{name: "utf8 spelling", env: map[string]string{"LANG": "C.utf8"}, want: false},
		{name: "C locale", env: map[string]string{"LANG": "C"}, want: true},
		{name: "latin1 ctype", env: map[string]string{"LC_CTYPE": "en_US.ISO8859-1", "LANG": "en_US.UTF-8"}, want: true},
		{name: "LC_ALL wins", env: map[string]string{"LC_ALL": "en_US.UTF-8", "LC_CTYPE": "C"}, want: false},
		{name: "dumb terminal", env: map[string]string{"TERM": "dumb", "LANG": "en_US.UTF-8"}, want: true},
		{name: "NIMBU_ASCII", env: map[string]string{"NIMBU_ASCII": "1", "LANG": "en_US.UTF-8"}, want: true},
		{name: "NIMBU_ASCII=0", env: map[string]string{"NIMBU_ASCII": "0"}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := asciiOnly(envMap(tc.env)); got != tc.want {
				t.Fatalf("asciiOnly() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnimationAllowed(t *testing.T) {
	if animationAllowed(false, true, envMap(map[string]string{"TERM": "dumb"})) {
		t.Fatal("TERM=dumb must not get cursor-moving animation")
	}
	if animationAllowed(true, true, envMap(nil)) {
		t.Fatal("--no-progress must disable animation")
	}
	if animationAllowed(false, false, envMap(nil)) {
		t.Fatal("non-terminal stderr must not animate")
	}
	if !animationAllowed(false, true, envMap(map[string]string{"TERM": "xterm-256color"})) {
		t.Fatal("terminal stderr should animate")
	}
}

func TestDumbTerminal(t *testing.T) {
	for term, want := range map[string]bool{"dumb": true, "DUMB": true, "xterm-256color": false, "": false} {
		if got := dumbTerminal(envMap(map[string]string{"TERM": term})); got != want {
			t.Fatalf("dumbTerminal(TERM=%q) = %v, want %v", term, got, want)
		}
	}
}

func TestThemeOffIsPassthrough(t *testing.T) {
	for _, theme := range []Theme{{}, NewTheme(termenv.Ascii)} {
		if theme.Enabled() {
			t.Fatal("expected disabled theme")
		}
		if got := theme.Error("x") + theme.Muted("y") + theme.Strong("z"); got != "xyz" {
			t.Fatalf("disabled theme must not style text, got %q", got)
		}
	}
}

func TestThemeOnStylesText(t *testing.T) {
	theme := NewTheme(termenv.ANSI256)
	if got := theme.Success("ok"); !strings.Contains(got, "\x1b[") || !strings.Contains(got, "ok") {
		t.Fatalf("expected styled text, got %q", got)
	}
}

func TestGlyphSetsAreComplete(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		g := GlyphSet(ascii)
		for name, value := range map[string]string{
			"OK": g.OK, "Fail": g.Fail, "Warn": g.Warn, "Arrow": g.Arrow, "Step": g.Step,
			"SubStep": g.SubStep, "Pending": g.Pending, "Top": g.Top, "Rail": g.Rail,
			"Bottom": g.Bottom, "Dash": g.Dash, "Ellipsis": g.Ellipsis,
		} {
			if value == "" {
				t.Fatalf("ascii=%v: glyph %s is empty", ascii, name)
			}
			if ascii && strings.IndexFunc(value, func(r rune) bool { return r > 127 }) >= 0 {
				t.Fatalf("ascii glyph %s = %q is not ASCII", name, value)
			}
		}
		if len(g.Spinner) == 0 {
			t.Fatalf("ascii=%v: empty spinner", ascii)
		}
	}
}

func TestCopyTimelineHoldsSpinnerBackForShortStages(t *testing.T) {
	tl, buf := newAnimatedCopyTimelineForTest(80, false)
	tl.spinnerDelay = time.Hour

	tl.StageStart("Pages")
	tl.StageItem("Pages", "about", 1, 3)
	if strings.Contains(buf.String(), "⠋") {
		t.Fatalf("spinner shown before the delay:\n%q", buf.String())
	}

	tl.StageDone("Pages", "3 copied")
	if !strings.Contains(buf.String(), "◇ Pages — 3 copied") {
		t.Fatalf("expected completed stage line, got %q", buf.String())
	}
}

func TestCopyTimelineASCIIGlyphs(t *testing.T) {
	tl, buf := newStaticCopyTimelineForTest()
	tl.glyphs = GlyphSet(true)

	tl.Header("a", "b")
	tl.StageDone("Pages", "3 copied")
	tl.StageWarning("Pages", "slow")
	tl.Footer()

	out := buf.String()
	if strings.IndexFunc(out, func(r rune) bool { return r > 127 }) >= 0 {
		t.Fatalf("expected ASCII-only timeline, got:\n%s", out)
	}
	for _, want := range []string{"+ Copying a -> b", "* Pages - 3 copied", "|  ! slow", "+ Done!"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in:\n%s", want, out)
		}
	}
}

func TestSyncTimelineHoldsSpinnerBackForShortCategories(t *testing.T) {
	var buf strings.Builder
	tl := &SyncTimeline{
		writer:       &buf,
		glyphs:       GlyphSet(false),
		animated:     true,
		spinnerDelay: time.Hour,
		activeCat:    -1,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
	tl.SetCategories([]SyncCategory{{Label: "layouts", Count: 1}})
	tl.StartCategory(0)
	tl.SetActiveFile("layouts/default.liquid")
	if buf.Len() != 0 {
		t.Fatalf("spinner shown before the delay: %q", buf.String())
	}

	tl.spinnerDelay = 0
	tl.SetActiveFile("layouts/default.liquid")
	if !strings.Contains(buf.String(), "layouts — 0/1  layouts/default.liquid") {
		t.Fatalf("expected live line once the delay passed, got %q", buf.String())
	}
}

// Every chrome token stays readable on its background: at least 3:1 (WCAG
// non-text and large/bold text) against a typical dark and light background.
func TestPaletteContrast(t *testing.T) {
	backgrounds := map[bool]string{true: "#1e1e1e", false: "#fafafa"}
	for dark, bg := range backgrounds {
		p := PaletteFor(dark)
		for name, hex := range map[string]string{
			"Success": p.Success, "Warning": p.Warning, "Error": p.Error, "Accent": p.Accent,
			"Muted": p.Muted, "Path": p.Path, "Border": p.Border,
		} {
			if hex == "" {
				t.Fatalf("dark=%v: %s is empty", dark, name)
			}
			if ratio := contrastRatio(t, hex, bg); ratio < 3 {
				t.Errorf("dark=%v: %s %s on %s has contrast %.2f, want >= 3", dark, name, hex, bg, ratio)
			}
		}
	}
	if PaletteFor(true) == PaletteFor(false) {
		t.Fatal("light and dark palettes must differ")
	}
}

func TestThemeForBackgroundUsesMatchingPalette(t *testing.T) {
	t.Setenv(ThemeEnv, "")
	t.Setenv("COLORFGBG", "")
	light := NewThemeFor(termenv.TrueColor, false)
	if light.Palette() != PaletteFor(false) {
		t.Fatal("light theme does not use the light palette")
	}
	if NewTheme(termenv.TrueColor).Palette() != PaletteFor(true) {
		t.Fatal("default theme must use the dark palette (background unknown)")
	}
	if got := light.Muted("x"); !strings.Contains(got, "38;2;71;85;105") {
		t.Fatalf("light muted = %q, want #475569", got)
	}
}

// termenv downsamples hex colors; a 256-color profile must never carry
// 24-bit sequences.
func TestThemeDownsamplesToProfile(t *testing.T) {
	theme := NewTheme(termenv.ANSI256)
	for _, got := range []string{theme.Success("a"), theme.Error("b"), theme.Paint("c", "#a78bfa", true), theme.Border("d")} {
		if strings.Contains(got, "38;2;") || !strings.Contains(got, "38;5;") {
			t.Fatalf("256-color theme emitted %q", got)
		}
	}
	if got := NewTheme(termenv.ANSI).Accent("e"); strings.Contains(got, "38;5;") || strings.Contains(got, "38;2;") {
		t.Fatalf("16-color theme emitted %q", got)
	}
}

func TestCharmProfileMapping(t *testing.T) {
	cases := map[termenv.Profile]colorprofile.Profile{
		termenv.TrueColor: colorprofile.TrueColor,
		termenv.ANSI256:   colorprofile.ANSI256,
		termenv.ANSI:      colorprofile.ANSI,
		termenv.Ascii:     colorprofile.ASCII,
	}
	for in, want := range cases {
		if got := CharmProfile(in); got != want {
			t.Errorf("CharmProfile(%v) = %v, want %v", in, got, want)
		}
	}
}

func contrastRatio(t *testing.T, a, b string) float64 {
	t.Helper()
	ca, err := colorful.Hex(a)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := colorful.Hex(b)
	if err != nil {
		t.Fatal(err)
	}
	lum := func(c colorful.Color) float64 {
		r, g, bl := c.LinearRgb()
		return 0.2126*r + 0.7152*g + 0.0722*bl
	}
	la, lb := lum(ca), lum(cb)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}
