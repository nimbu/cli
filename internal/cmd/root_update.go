package cmd

import (
	"context"
	"io"
	"strings"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/updatecheck"
)

var notifyUpdate = updatecheck.MaybeNotify

var stderrIsTTY = func(w *output.Writer) bool {
	return w != nil && w.ErrIsTTY()
}

var styleUpdateNotice = func(w *output.Writer) updatecheck.Style {
	if w == nil {
		return updatecheck.Style{}
	}

	useColor := w.Color == "always" || (w.Color != "never" && stderrIsTTY(w))
	if !useColor {
		return updatecheck.Style{}
	}

	return updatecheck.Style{
		Bold: func(s string) string { return "\x1b[1m" + s + "\x1b[0m" },
		Dim:  func(s string) string { return "\x1b[2m" + s + "\x1b[0m" },
	}
}

func maybeNotifyUpdate(ctx context.Context, commandPath string, flags RootFlags, currentVersion string) {
	if flags.JSON || flags.Plain || flags.NoInput {
		return
	}
	if currentVersion == "" || currentVersion == "dev" {
		return
	}
	if commandPath == "completion" || strings.HasPrefix(commandPath, "completion ") {
		return
	}

	writer := output.WriterFromContext(ctx)
	if !stderrIsTTY(writer) {
		return
	}

	var errWriter io.Writer
	if writer != nil {
		errWriter = writer.Err
	}

	notifyUpdate(ctx, errWriter, currentVersion, styleUpdateNotice(writer))
}
