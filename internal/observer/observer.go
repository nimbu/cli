package observer

import "context"

// CopyObserver receives progress events during copy operations.
// Implementations must be safe for concurrent use.
type CopyObserver interface {
	StageStart(name string)
	StageItem(name, detail string, current, total int64)
	StageDone(name, summary string)
	StageSkip(name, reason string)
	SubStageDone(stage, sub, summary string)
	StageWarning(stage, msg string)
}

type observerCtxKey struct{}

// WithCopyObserver stores a CopyObserver in the context.
func WithCopyObserver(ctx context.Context, obs CopyObserver) context.Context {
	return context.WithValue(ctx, observerCtxKey{}, obs)
}

// ObserverFromContext returns the CopyObserver from context, or a no-op observer.
func ObserverFromContext(ctx context.Context) CopyObserver {
	if obs, ok := ctx.Value(observerCtxKey{}).(CopyObserver); ok && obs != nil {
		return obs
	}
	return nopObserver{}
}

type nopObserver struct{}

func (nopObserver) StageStart(string)                      {}
func (nopObserver) StageItem(string, string, int64, int64) {}
func (nopObserver) StageDone(string, string)               {}
func (nopObserver) StageSkip(string, string)               {}
func (nopObserver) SubStageDone(string, string, string)    {}
func (nopObserver) StageWarning(string, string)            {}
