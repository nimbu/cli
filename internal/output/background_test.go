package output

import (
	"testing"

	"github.com/muesli/termenv"
)

func TestBackgroundIsDarkFromEnv(t *testing.T) {
	for _, tc := range []struct {
		theme, fgbg string
		dark        bool
	}{
		{"", "", true},
		{"light", "", false},
		{" LIGHT ", "0;0", false},
		{"dark", "0;15", true},
		{"", "0;15", false},
		{"", "0;7", false},
		{"", "0;default;15", false},
		{"", "15;0", true},
		{"", "7;8", true},
		{"", "garbage", true},
		{"bogus", "0;15", false},
	} {
		env := map[string]string{ThemeEnv: tc.theme, "COLORFGBG": tc.fgbg}
		if got := backgroundIsDarkFromEnv(func(k string) string { return env[k] }); got != tc.dark {
			t.Fatalf("NIMBU_THEME=%q COLORFGBG=%q: dark = %v, want %v", tc.theme, tc.fgbg, got, tc.dark)
		}
	}
}

func TestLineThemeFollowsLightBackground(t *testing.T) {
	t.Setenv(ThemeEnv, "")
	t.Setenv("COLORFGBG", "0;15")
	if NewTheme(termenv.TrueColor).Palette() != PaletteFor(false) {
		t.Fatal("COLORFGBG with a light background must select the light palette")
	}
	t.Setenv(ThemeEnv, "dark")
	if NewTheme(termenv.TrueColor).Palette() != PaletteFor(true) {
		t.Fatal("NIMBU_THEME=dark must win over COLORFGBG")
	}
}
