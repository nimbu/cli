package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
)

// unsetEmptyFlagEnv removes environment variables bound to flags (NIMBU_JSON,
// NIMBU_NO_INPUT, NIMBU_TIMEOUT, ...) whose value is empty or blank, so
// `NIMBU_JSON=` means "not set" instead of failing every command with a parse
// error. Kong reads these with os.LookupEnv, which treats empty as set.
func unsetEmptyFlagEnv(app *kong.Application) {
	if app == nil {
		return
	}
	_ = kong.Visit(app, func(node kong.Visitable, next kong.Next) error {
		if flag, ok := node.(*kong.Flag); ok {
			for _, name := range flag.Envs {
				if value, set := os.LookupEnv(name); set && strings.TrimSpace(value) == "" {
					_ = os.Unsetenv(name)
				}
			}
		}
		return next(nil)
	})
}

var colorModes = map[string]bool{"auto": true, "always": true, "never": true}

// normalizeColorEnv makes NIMBU_COLOR case- and space-insensitive, like the
// help printer and the output package read it. A blank value means unset; an
// unknown one falls back to auto with a warning instead of failing every
// command (tab completion included) with an error that blames --color.
func normalizeColorEnv() (warning string) {
	value, set := os.LookupEnv("NIMBU_COLOR")
	if !set {
		return ""
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch {
	case normalized == "":
		_ = os.Unsetenv("NIMBU_COLOR")
	case colorModes[normalized]:
		_ = os.Setenv("NIMBU_COLOR", normalized)
	default:
		_ = os.Unsetenv("NIMBU_COLOR")
		return fmt.Sprintf("ignoring NIMBU_COLOR=%q: use auto, always or never", value)
	}
	return ""
}

// normalizeColorArgs lower-cases an explicit --color value, so --color=Never
// works as it does in help; unknown values still fail the enum.
func normalizeColorArgs(args []string) []string {
	out := append([]string(nil), args...)
	for i := 0; i < len(out); i++ {
		switch arg := out[i]; {
		case arg == "--":
			return out
		case arg == "--color" && i+1 < len(out):
			out[i+1] = strings.ToLower(strings.TrimSpace(out[i+1]))
			i++
		case strings.HasPrefix(arg, "--color="):
			out[i] = "--color=" + strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--color=")))
		}
	}
	return out
}
