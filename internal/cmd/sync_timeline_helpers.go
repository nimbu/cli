package cmd

import (
	"context"

	"github.com/nimbu/cli/internal/output"
)

// syncWithTimeline wraps a push/sync operation with timeline UI for human mode.
// Returns the augmented context and the timeline (nil if not human mode).
func syncWithTimeline(ctx context.Context, mode, theme string, dryRun bool) (context.Context, *output.SyncTimeline) {
	if !output.IsHuman(ctx) {
		return ctx, nil
	}
	tl := output.NewSyncTimeline(ctx, mode, theme, dryRun)
	ctx = output.WithSyncTimeline(ctx, tl)
	ctx = output.WithProgress(ctx, output.NewDisabledProgress())
	return ctx, tl
}

// finishSyncTimelineError marks an error the timeline already rendered, so
// emitCommandError skips the duplicate print. An error from before the
// timeline drew anything (build, selectors, planning) is returned as is.
func finishSyncTimelineError(tl *output.SyncTimeline, rendered bool, err error) error {
	if tl == nil || !rendered {
		return err
	}
	return &displayedError{err: err}
}
