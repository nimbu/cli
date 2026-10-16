package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/nimbu/cli/internal/output"
)

// requestedJSON reports whether the invocation asked for JSON, read from the
// raw args and NIMBU_JSON so it also works when flag parsing failed.
func requestedJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" {
			return true
		}
		if value, ok := strings.CutPrefix(arg, "--json="); ok {
			if on, err := strconv.ParseBool(value); err == nil {
				return on
			}
		}
	}
	return envTruthy(os.Getenv("NIMBU_JSON"))
}

func envTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// emitEarlyError reports a failure that happens before the command context
// exists (flag parsing, conflicting output modes) through the same error
// contract as command failures: the JSON envelope when JSON was requested,
// else "error:"/"hint:" lines.
func emitEarlyError(jsonMode bool, color string, err error) error {
	mode := output.Mode{JSON: jsonMode}
	writer := output.DefaultWriter()
	writer.Mode = mode
	writer.Color = validColor(color)
	ctx := output.WithWriter(output.WithMode(context.Background(), mode), writer)
	return emitCommandError(ctx, err)
}

func validColor(color string) string {
	switch color {
	case "always", "never":
		return color
	}
	return "auto"
}

// parseUsageError turns a Kong parse failure into a typed usage error whose
// hint points at the help of the command being parsed.
func parseUsageError(err error) error {
	help := "nimbu --help"
	var parseErr *kong.ParseError
	if errors.As(err, &parseErr) && parseErr.Context != nil {
		if node := parseErr.Context.Selected(); node != nil {
			help = node.FullPath() + " --help"
		}
	}
	return newHintedError(err, errorUsageInvalid, ExitUsage, fmt.Sprintf("run `%s` for usage", help))
}

// writeRootWarning prints a warning raised before the command runs. In JSON
// mode it is a one-line JSON object, so stderr stays machine-readable next to
// the error envelope.
func writeRootWarning(w io.Writer, jsonMode bool, message string) {
	if !jsonMode {
		_, _ = fmt.Fprintf(w, "warning: %s\n", message)
		return
	}
	data, err := json.Marshal(map[string]any{
		"status":  "warning",
		"warning": map[string]string{"message": message},
	})
	if err != nil {
		_, _ = fmt.Fprintf(w, "warning: %s\n", message)
		return
	}
	_, _ = fmt.Fprintf(w, "%s\n", data)
}
