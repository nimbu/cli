package output

import (
	"os"
	"strconv"
	"strings"
)

// ThemeEnv selects the line-output palette: "light" or "dark". Empty or any
// other value means unset.
const ThemeEnv = "NIMBU_THEME"

// lineBackgroundIsDark picks the palette for line output from cheap signals
// only; line output never queries the terminal. NIMBU_THEME wins, then the
// background index in COLORFGBG ("fg;bg", set by rxvt, Konsole, iTerm2 and
// others), then the dark default.
func lineBackgroundIsDark() bool {
	return backgroundIsDarkFromEnv(os.Getenv)
}

func backgroundIsDarkFromEnv(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(ThemeEnv))) {
	case "light":
		return false
	case "dark":
		return true
	}
	if dark, ok := colorFGBGIsDark(getenv("COLORFGBG")); ok {
		return dark
	}
	return true
}

// colorFGBGIsDark reads the background ANSI index, the last field of
// COLORFGBG: 7 (white) and 9-15 (bright colors) are light, 0-6 and 8 dark.
func colorFGBGIsDark(value string) (dark, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return false, false
	}
	fields := strings.Split(value, ";")
	bg, err := strconv.Atoi(strings.TrimSpace(fields[len(fields)-1]))
	if err != nil || bg < 0 || bg > 15 {
		return false, false
	}
	return bg <= 6 || bg == 8, true
}
