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

// finishSyncTimelineError wraps an error that was already rendered by the timeline.
func finishSyncTimelineError(tl *output.SyncTimeline, err error) error {
	if tl == nil {
		return err
	}
	return &displayedError{err: err}
}
