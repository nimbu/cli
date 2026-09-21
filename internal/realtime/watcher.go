package realtime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/nimbu/cli/internal/api"
)

// Defaults for the reconnect loop.
const (
	DefaultMaxAttempts = 10
	DefaultBackoffBase = time.Second
	DefaultBackoffMax  = 30 * time.Second
)

// BackoffOptions tunes the delay between reconnect attempts.
type BackoffOptions struct {
	Base   time.Duration
	Max    time.Duration
	Jitter func(time.Duration) time.Duration
}

// WatchOptions configures the reconnecting watch loop.
type WatchOptions struct {
	Identifier  Identifier
	MintGrant   func(ctx context.Context) (string, error)
	Dial        func(ctx context.Context, grant string) (Conn, error)
	Handler     Handler
	Once        bool
	Timeout     time.Duration
	PingTimeout time.Duration
	MaxAttempts int
	Backoff     BackoffOptions
	Now         func() time.Time
	Sleep       func(ctx context.Context, d time.Duration) error
}

// ExitReason explains why Watch returned.
type ExitReason int

const (
	// ExitInterrupted means the caller's context was cancelled.
	ExitInterrupted ExitReason = iota
	// ExitTimeout means the configured watch duration elapsed.
	ExitTimeout
	// ExitOnce means the first event arrived and Once was set.
	ExitOnce
	// ExitFatal means watching stopped on an error that will not resolve.
	ExitFatal
)

// String renders the exit reason for logs.
func (r ExitReason) String() string {
	switch r {
	case ExitInterrupted:
		return "interrupted"
	case ExitTimeout:
		return "timeout"
	case ExitOnce:
		return "once"
	case ExitFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// Watch mints grants, dials and runs sessions until the context ends, the
// first event arrives under Once, or the attempt budget runs out.
func Watch(ctx context.Context, opts WatchOptions) (ExitReason, error) {
	if opts.MintGrant == nil {
		return ExitFatal, errors.New("realtime: watch needs a grant minter")
	}
	if opts.Dial == nil {
		return ExitFatal, errors.New("realtime: watch needs a dialer")
	}
	if opts.Handler == nil {
		opts.Handler = nopHandler{}
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = DefaultMaxAttempts
	}
	if opts.Backoff.Base <= 0 {
		opts.Backoff.Base = DefaultBackoffBase
	}
	if opts.Backoff.Max <= 0 {
		opts.Backoff.Max = DefaultBackoffMax
	}
	if opts.Backoff.Jitter == nil {
		opts.Backoff.Jitter = defaultJitter
	}
	if opts.Sleep == nil {
		opts.Sleep = sleepContext
	}

	parent := ctx
	runCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	w := &watcher{
		opts:   opts,
		dedupe: NewDeduper(DefaultDedupeCapacity),
	}

	reason, err := w.loop(runCtx)
	if w.onceFired {
		return ExitOnce, nil
	}
	if parent.Err() != nil {
		return ExitInterrupted, nil
	}
	if runCtx.Err() != nil {
		return ExitTimeout, nil
	}
	return reason, err
}

type watcher struct {
	opts      WatchOptions
	dedupe    *Deduper
	attempts  int
	onceFired bool
}

func (w *watcher) loop(ctx context.Context) (ExitReason, error) {
	for {
		if ctx.Err() != nil {
			return ExitInterrupted, nil
		}

		grant, err := w.opts.MintGrant(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ExitInterrupted, nil
			}
			if !api.IsRateLimit(err) {
				return ExitFatal, err
			}
			if retry, reason, retryErr := w.retry(ctx, err); !retry {
				return reason, retryErr
			}
			continue
		}

		conn, err := w.opts.Dial(ctx, grant)
		if err != nil {
			if ctx.Err() != nil {
				return ExitInterrupted, nil
			}
			if retry, reason, retryErr := w.retry(ctx, err); !retry {
				return reason, retryErr
			}
			continue
		}

		result, err := w.session(ctx, conn)
		if w.onceFired {
			return ExitOnce, nil
		}
		if ctx.Err() != nil {
			return ExitInterrupted, nil
		}

		var subErr *SubscriptionError
		if errors.As(err, &subErr) && subErr.Fatal() {
			return ExitFatal, err
		}
		if result == ResultDone {
			return ExitInterrupted, nil
		}

		if retry, reason, retryErr := w.retry(ctx, err); !retry {
			return reason, retryErr
		}
	}
}

func (w *watcher) session(ctx context.Context, conn Conn) (SessionResult, error) {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	handler := &watchHandler{
		inner:     w.opts.Handler,
		once:      w.opts.Once,
		stop:      cancel,
		onConfirm: func() { w.attempts = 0 },
		fired:     &w.onceFired,
	}

	return RunSession(sessionCtx, conn, SessionOptions{
		Identifier:  w.opts.Identifier,
		PingTimeout: w.opts.PingTimeout,
		Now:         w.opts.Now,
		Dedupe:      w.dedupe,
		Handler:     handler,
	})
}

// retry accounts for one failed attempt and waits out the backoff. It reports
// false when the attempt budget is spent or the wait was interrupted.
func (w *watcher) retry(ctx context.Context, cause error) (bool, ExitReason, error) {
	w.attempts++
	if w.attempts > w.opts.MaxAttempts {
		if cause == nil {
			cause = fmt.Errorf("realtime: giving up after %d attempts", w.opts.MaxAttempts)
		}
		return false, ExitFatal, cause
	}

	w.opts.Handler.OnStatus(fmt.Sprintf("reconnecting (attempt %d)", w.attempts))
	if err := w.opts.Sleep(ctx, w.backoff(w.attempts)); err != nil {
		return false, ExitInterrupted, nil
	}
	return true, ExitInterrupted, nil
}

func (w *watcher) backoff(attempt int) time.Duration {
	delay := time.Duration(float64(w.opts.Backoff.Base) * math.Pow(2, float64(attempt-1)))
	if delay > w.opts.Backoff.Max || delay <= 0 {
		delay = w.opts.Backoff.Max
	}
	if w.opts.Backoff.Jitter != nil {
		delay = w.opts.Backoff.Jitter(delay)
	}
	return delay
}

type watchHandler struct {
	inner     Handler
	once      bool
	stop      context.CancelFunc
	onConfirm func()
	fired     *bool
}

func (h *watchHandler) OnEvent(ev Event) {
	h.inner.OnEvent(ev)
	if h.once && ev.Event != EventResync && !*h.fired {
		*h.fired = true
		h.stop()
	}
}

func (h *watchHandler) OnControl(c Control) {
	h.inner.OnControl(c)
	if c.Type == ControlConfirmSubscribe || c.Type == ControlSubscribed {
		h.onConfirm()
	}
}

func (h *watchHandler) OnStatus(msg string) {
	h.inner.OnStatus(msg)
}

// defaultJitter spreads reconnects between half and the full delay so a fleet
// of watchers does not stampede the server after an outage.
func defaultJitter(d time.Duration) time.Duration {
	if d <= 1 {
		return d
	}
	half := d / 2
	return half + rand.N(d-half)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
