package cmd

import (
	"context"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// copyWithTimeline wraps a copy operation with timeline UI for human mode.
// Returns the augmented context and the timeline (nil if not human mode).
func copyWithTimeline(ctx context.Context, stageName, from, to string, dryRun bool) (context.Context, *output.CopyTimeline) {
	if !output.IsHuman(ctx) {
		return ctx, nil
	}
	tl := output.NewCopyTimeline(ctx, dryRun)
	ctx = migrate.WithCopyObserver(ctx, tl)
	ctx = output.WithProgress(ctx, output.NewDisabledProgress())
	tl.Header(from, to)
	tl.StageStart(stageName)
	return ctx, tl
}

// finishCopyTimeline finalizes a single-stage timeline with success footer.
func finishCopyTimeline(tl *output.CopyTimeline, stageName, summary string) {
	if tl == nil {
		return
	}
	tl.StageDone(stageName, summary)
	tl.Footer()
}

// finishCopyTimelineError finalizes the timeline with an error footer.
// Returns a displayedError wrapper so emitCommandError skips the duplicate print.
func finishCopyTimelineError(tl *output.CopyTimeline, err error) error {
	if tl == nil {
		return err
	}
	tl.ErrorFooter(err.Error())
	return &displayedError{err: err}
}
