package migrate

import (
	"context"

	"github.com/nimbu/cli/internal/observer"
)

// CopyObserver is an alias for the shared observer interface.
type CopyObserver = observer.CopyObserver

// WithCopyObserver stores a CopyObserver in the context.
func WithCopyObserver(ctx context.Context, obs CopyObserver) context.Context {
	return observer.WithCopyObserver(ctx, obs)
}

// ObserverFromContext returns the CopyObserver from context, or a no-op observer.
func ObserverFromContext(ctx context.Context) CopyObserver {
	return observer.ObserverFromContext(ctx)
}

func emitStageStart(ctx context.Context, name string) {
	observer.ObserverFromContext(ctx).StageStart(name)
}

func emitStageItem(ctx context.Context, name, detail string, current, total int64) {
	observer.ObserverFromContext(ctx).StageItem(name, detail, current, total)
}

func emitStageDone(ctx context.Context, name, summary string) {
	observer.ObserverFromContext(ctx).StageDone(name, summary)
}

func emitStageSkip(ctx context.Context, name, reason string) {
	observer.ObserverFromContext(ctx).StageSkip(name, reason)
}

func emitSubStageDone(ctx context.Context, stage, sub, summary string) {
	observer.ObserverFromContext(ctx).SubStageDone(stage, sub, summary)
}

func emitStageWarning(ctx context.Context, stage, msg string) {
	observer.ObserverFromContext(ctx).StageWarning(stage, msg)
}
