package cmd

import (
	"context"
	"io"
	"strings"

	"github.com/muesli/termenv"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/updatecheck"
)

var (
	notifyUpdate   = updatecheck.MaybeNotify
	prefetchUpdate = updatecheck.Prefetch
)

var stderrIsTTY = func(w *output.Writer) bool {
	return w != nil && w.ErrIsTTY()
}

// styleUpdateNotice styles the notice with bold and faint attributes only, so
// it reads on light and dark terminals alike. The notice goes to stderr, so
// stderr's color decision applies.
var styleUpdateNotice = func(w *output.Writer) updatecheck.Style {
	if w == nil || !output.ColorEnabled(w.Color, stderrIsTTY(w)) {
		return updatecheck.Style{}
	}
	theme := output.NewTheme(termenv.ANSI)
	return updatecheck.Style{Bold: theme.Strong, Dim: theme.Faint}
}

// updateNoticeWanted reports whether this invocation may show an update
// notice: a human at a terminal running a real release build.
func updateNoticeWanted(ctx context.Context, commandPath string, flags RootFlags, currentVersion string) bool {
	if flags.JSON || flags.Plain || flags.NoInput {
		return false
	}
	if currentVersion == "" || currentVersion == "dev" {
		return false
	}
	if commandPath == "completion" || strings.HasPrefix(commandPath, "completion ") || commandPath == "__complete" || strings.HasPrefix(commandPath, "__complete ") {
		return false
	}
	return stderrIsTTY(output.WriterFromContext(ctx))
}

// prefetchUpdateCheck starts a stale update-cache refresh before the command
// runs, so the network call overlaps the command rather than delaying exit.
func prefetchUpdateCheck(ctx context.Context, commandPath string, flags RootFlags, currentVersion string) {
	if !updateNoticeWanted(ctx, commandPath, flags, currentVersion) {
		return
	}
	prefetchUpdate(ctx, currentVersion)
}

func maybeNotifyUpdate(ctx context.Context, commandPath string, flags RootFlags, currentVersion string) {
	if !updateNoticeWanted(ctx, commandPath, flags, currentVersion) {
		return
	}

	writer := output.WriterFromContext(ctx)
	var errWriter io.Writer
	if writer != nil {
		errWriter = writer.Err
	}

	notifyUpdate(ctx, errWriter, currentVersion, styleUpdateNotice(writer))
}
