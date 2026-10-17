package cmd

import (
	"os"
	"strings"
	"testing"
)

// ambientTestEnv lists the variables from the developer's shell that change
// how commands render or behave. Tests that need one set it with t.Setenv.
var ambientTestEnv = []string{
	// Output modes and prompts.
	"NIMBU_JSON", "NIMBU_PLAIN", "NIMBU_NO_INPUT", "NIMBU_COLOR", "NIMBU_WIDE",
	"NIMBU_NO_PROGRESS", "NIMBU_THEME", "NO_COLOR",
	// Glyphs: output.ASCIIOnly reads these.
	"NIMBU_ASCII", "LC_ALL", "LC_CTYPE", "LANG",
	// Target and credentials.
	"NIMBU_SITE", "NIMBU_API_URL", "NIMBU_TIMEOUT", "NIMBU_READONLY", "NIMBU_TOKEN",
	"NIMBU_PROJECT_DIR",
}

// TestMain runs the package tests in a predictable environment, so they pass
// the same way in a shell with TERM=dumb, LC_ALL=C or NIMBU_JSON=1.
func TestMain(m *testing.M) {
	// Re-executed helper processes get their environment from the parent
	// test, which already ran this cleanup.
	if os.Getenv("NIMBU_SERVER_SIGNAL_HELPER") == "" {
		for _, key := range ambientTestEnv {
			_ = os.Unsetenv(key)
		}
		if strings.EqualFold(os.Getenv("TERM"), "dumb") {
			_ = os.Setenv("TERM", "xterm-256color")
		}
	}
	os.Exit(m.Run())
}
