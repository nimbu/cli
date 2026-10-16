package cmd

import (
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
