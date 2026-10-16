package output

import (
	"io"
	"os"
	"strings"

	"github.com/muesli/termenv"
)

// ColorEnabled decides whether a stream gets color.
//
// Precedence, highest first:
//  1. --color=always|never, also set through NIMBU_COLOR (mode)
//  2. NO_COLOR (non-empty) disables
//  3. TERM=dumb disables
//  4. FORCE_COLOR=0 disables
//  5. whether the stream is a terminal
//
// Generic force signals (FORCE_COLOR=1, CLICOLOR_FORCE) are deliberately not
// honored: they are commonly exported shell-wide for Node tooling and would
// leak ANSI escapes into pipes and agent transcripts. Use --color=always or
// NIMBU_COLOR=always to force color.
func ColorEnabled(mode string, isTTY bool) bool {
	return colorEnabled(mode, isTTY, os.Getenv)
}

func colorEnabled(mode string, isTTY bool, getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "always":
		return true
	case "never":
		return false
	}
	if getenv("NO_COLOR") != "" {
		return false
	}
	if strings.EqualFold(getenv("TERM"), "dumb") {
		return false
	}
	if getenv("FORCE_COLOR") == "0" {
		return false
	}
	return isTTY
}

// UseColor reports whether stdout should be colored.
func (w *Writer) UseColor() bool {
	return ColorEnabled(w.Color, w.IsTTY())
}

// ErrUseColor reports whether stderr should be colored. Use this for chrome
// (progress, timelines, banners, prompts) that is written to w.Err.
func (w *Writer) ErrUseColor() bool {
	return ColorEnabled(w.Color, w.ErrIsTTY())
}

// OutColorProfile returns the termenv profile for stdout: Ascii when color is
// off, otherwise the richest profile the environment advertises.
func (w *Writer) OutColorProfile() termenv.Profile {
	return colorProfile(w.UseColor(), w.Out)
}

// ErrColorProfile returns the termenv profile for stderr.
func (w *Writer) ErrColorProfile() termenv.Profile {
	return colorProfile(w.ErrUseColor(), w.Err)
}

func colorProfile(enabled bool, stream io.Writer) termenv.Profile {
	if !enabled {
		return termenv.Ascii
	}
	if stream == nil {
		stream = os.Stdout
	}
	// Color was already decided above, so treat the stream as a terminal and
	// only ask the environment (TERM/COLORTERM) how rich the palette may be.
	profile := termenv.NewOutput(stream, termenv.WithUnsafe()).ColorProfile()
	if profile == termenv.Ascii {
		// Forced color on a stream with no TERM hint: basic ANSI is the safe floor.
		return termenv.ANSI
	}
	return profile
}
