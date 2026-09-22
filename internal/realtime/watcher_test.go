package realtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/api"
)

type watchHarness struct {
	mu      sync.Mutex
	grants  []string
	sleeps  []time.Duration
	scripts [][]string
	dialErr error
	handler *recordingHandler
}

func (h *watchHarness) mint(context.Context) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	grant := "grant-" + string(rune('a'+len(h.grants)))
	h.grants = append(h.grants, grant)
	return grant, nil
}

func (h *watchHarness) dial(context.Context, string) (Conn, error) {
	if h.dialErr != nil {
		return nil, h.dialErr
	}
	h.mu.Lock()
	var frames []string
	if len(h.scripts) > 0 {
		frames = h.scripts[0]
		if len(h.scripts) > 1 {
			h.scripts = h.scripts[1:]
		}
	}
	h.mu.Unlock()
	return newFakeConn(frames...), nil
}

func (h *watchHarness) sleep(_ context.Context, d time.Duration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sleeps = append(h.sleeps, d)
	return nil
}

func (h *watchHarness) grantCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.grants)
}

func (h *watchHarness) options() WatchOptions {
	h.handler = &recordingHandler{}
	return WatchOptions{
		Identifier:  NewIdentifier("blog", nil),
		MintGrant:   h.mint,
		Dial:        h.dial,
		Handler:     h.handler,
		PingTimeout: 50 * time.Millisecond,
		MaxAttempts: 3,
		Backoff:     BackoffOptions{Base: time.Second, Max: 8 * time.Second, Jitter: identityJitter},
		Sleep:       h.sleep,
		Now:         func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
}

func TestWatchRetriesWithFreshGrants(t *testing.T) {
	h := &watchHarness{dialErr: errors.New("connection refused")}
	reason, err := Watch(context.Background(), h.options())

	if reason != ExitFatal {
		t.Fatalf("reason = %v, want ExitFatal", reason)
	}
	if err == nil || err.Error() != "connection refused" {
		t.Fatalf("err = %v, want the dial error", err)
	}
	if h.grantCount() != 4 {
		t.Fatalf("grants minted = %d, want 4 (one per attempt)", h.grantCount())
	}
	if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}; !equalDurations(h.sleeps, want) {
		t.Fatalf("sleeps = %v, want %v", h.sleeps, want)
	}
}

func TestWatchBackoffIsCappedAndJittered(t *testing.T) {
	h := &watchHarness{dialErr: errors.New("boom")}
	opts := h.options()
	opts.MaxAttempts = 5
	opts.Backoff = BackoffOptions{
		Base: time.Second,
		Max:  4 * time.Second,
		Jitter: func(d time.Duration) time.Duration {
			return d / 2
		},
	}

	if reason, _ := Watch(context.Background(), opts); reason != ExitFatal {
		t.Fatalf("reason = %v, want ExitFatal", reason)
	}
	want := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 2 * time.Second, 2 * time.Second}
	if !equalDurations(h.sleeps, want) {
		t.Fatalf("sleeps = %v, want %v", h.sleeps, want)
	}
}

func TestWatchFatalMintError(t *testing.T) {
	h := &watchHarness{}
	opts := h.options()
	opts.MintGrant = func(context.Context) (string, error) {
		return "", &api.Error{StatusCode: 403, Message: "realtime disabled"}
	}

	reason, err := Watch(context.Background(), opts)
	if reason != ExitFatal || err == nil {
		t.Fatalf("reason = %v, err = %v, want ExitFatal", reason, err)
	}
	if len(h.sleeps) != 0 {
		t.Fatalf("sleeps = %v, want none", h.sleeps)
	}
}

func TestWatchRetriesRateLimitedMint(t *testing.T) {
	h := &watchHarness{}
	opts := h.options()
	opts.MaxAttempts = 2
	opts.MintGrant = func(context.Context) (string, error) {
		return "", &api.Error{StatusCode: 429, Message: "rate limit exceeded"}
	}

	reason, _ := Watch(context.Background(), opts)
	if reason != ExitFatal {
		t.Fatalf("reason = %v, want ExitFatal", reason)
	}
	if len(h.sleeps) != 2 {
		t.Fatalf("sleeps = %v, want 2 retries", h.sleeps)
	}
}

func TestWatchInvalidQueryIsFatalWithoutRetry(t *testing.T) {
	h := &watchHarness{scripts: [][]string{{
		`{"type":"welcome"}`,
		`{"identifier":"x","message":{"type":"subscription_error","code":"invalid_query"}}`,
	}}}
	opts := h.options()

	reason, err := Watch(context.Background(), opts)
	if reason != ExitFatal {
		t.Fatalf("reason = %v, want ExitFatal", reason)
	}
	var subErr *SubscriptionError
	if !errors.As(err, &subErr) || !subErr.Fatal() {
		t.Fatalf("err = %v, want fatal *SubscriptionError", err)
	}
	if len(h.sleeps) != 0 {
		t.Fatalf("sleeps = %v, want none", h.sleeps)
	}
	if h.grantCount() != 1 {
		t.Fatalf("grants = %d, want 1", h.grantCount())
	}
}

func TestWatchRetryableSubscriptionErrorReconnects(t *testing.T) {
	script := []string{
		`{"type":"welcome"}`,
		`{"identifier":"x","message":{"type":"subscription_error","code":"not_available"}}`,
	}
	h := &watchHarness{scripts: [][]string{script}}
	opts := h.options()
	opts.MaxAttempts = 2

	reason, err := Watch(context.Background(), opts)
	if reason != ExitFatal {
		t.Fatalf("reason = %v, want ExitFatal after the attempt cap", reason)
	}
	var subErr *SubscriptionError
	if !errors.As(err, &subErr) || subErr.Code != "not_available" {
		t.Fatalf("err = %v, want not_available", err)
	}
	if h.grantCount() != 3 {
		t.Fatalf("grants = %d, want 3", h.grantCount())
	}
}

func TestWatchConfirmResetsBackoff(t *testing.T) {
	confirmed := []string{
		`{"type":"welcome"}`,
		`{"type":"confirm_subscription","identifier":"x"}`,
		`{"type":"disconnect","reason":"server_restart"}`,
	}
	h := &watchHarness{scripts: [][]string{confirmed}}
	opts := h.options()
	opts.MaxAttempts = 2

	// A session that confirms is progress, so the loop never exhausts its
	// budget. Stop it from the outside after a few reconnects.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts.Sleep = func(_ context.Context, d time.Duration) error {
		h.mu.Lock()
		h.sleeps = append(h.sleeps, d)
		count := len(h.sleeps)
		h.mu.Unlock()
		if count >= 4 {
			cancel()
			return context.Canceled
		}
		return nil
	}

	reason, err := Watch(ctx, opts)
	if reason != ExitInterrupted || err != nil {
		t.Fatalf("reason = %v, err = %v, want ExitInterrupted", reason, err)
	}
	for _, sleep := range h.sleeps {
		if sleep != time.Second {
			t.Fatalf("sleeps = %v, want every delay reset to the base", h.sleeps)
		}
	}
}

func TestWatchOnceStopsAfterFirstEvent(t *testing.T) {
	h := &watchHarness{scripts: [][]string{{
		`{"type":"welcome"}`,
		`{"type":"confirm_subscription","identifier":"x"}`,
		`{"identifier":"x","message":{"event_id":"ev_1","event":"resync","id":""}}`,
		`{"identifier":"x","message":{"event_id":"ev_2","event":"added","id":"1"}}`,
	}}}
	opts := h.options()
	opts.Once = true

	reason, err := Watch(context.Background(), opts)
	if reason != ExitOnce || err != nil {
		t.Fatalf("reason = %v, err = %v, want ExitOnce", reason, err)
	}
	h.handler.mu.Lock()
	defer h.handler.mu.Unlock()
	if len(h.handler.events) != 2 {
		t.Fatalf("events = %d, want the resync plus the added event", len(h.handler.events))
	}
	if h.handler.events[1].Event != EventAdded {
		t.Fatalf("last event = %q, want added", h.handler.events[1].Event)
	}
}

func TestWatchTimeoutExitsCleanly(t *testing.T) {
	h := &watchHarness{scripts: [][]string{{`{"type":"welcome"}`, `{"type":"confirm_subscription","identifier":"x"}`}}}
	opts := h.options()
	opts.PingTimeout = time.Second
	opts.Timeout = 30 * time.Millisecond

	reason, err := Watch(context.Background(), opts)
	if reason != ExitTimeout || err != nil {
		t.Fatalf("reason = %v, err = %v, want ExitTimeout", reason, err)
	}
}

func TestWatchInterruptExitsCleanly(t *testing.T) {
	h := &watchHarness{scripts: [][]string{{`{"type":"welcome"}`, `{"type":"confirm_subscription","identifier":"x"}`}}}
	opts := h.options()
	opts.PingTimeout = time.Second

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	reason, err := Watch(ctx, opts)
	if reason != ExitInterrupted || err != nil {
		t.Fatalf("reason = %v, err = %v, want ExitInterrupted", reason, err)
	}
}

func equalDurations(got, want []time.Duration) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestDefaultJitterStaysWithinHalfToFull(t *testing.T) {
	for range 200 {
		got := defaultJitter(10 * time.Second)
		if got < 5*time.Second || got > 10*time.Second {
			t.Fatalf("jitter out of range: %s", got)
		}
	}
	if got := defaultJitter(0); got != 0 {
		t.Fatalf("zero delay should stay zero, got %s", got)
	}
}

func identityJitter(d time.Duration) time.Duration { return d }
