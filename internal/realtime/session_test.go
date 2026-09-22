package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

const testTick = 5 * time.Millisecond

var errFakeConnClosed = errors.New("fake conn closed")

// fakeConn is a scripted in-memory Conn: tests push frames and inspect writes.
type fakeConn struct {
	frames    chan []byte
	closed    chan struct{}
	closeOnce sync.Once

	mu      sync.Mutex
	writes  []string
	reading int
}

func newFakeConn(frames ...string) *fakeConn {
	c := &fakeConn{
		frames: make(chan []byte, 32),
		closed: make(chan struct{}),
	}
	for _, frame := range frames {
		c.push(frame)
	}
	return c
}

func (c *fakeConn) push(frame string) {
	c.frames <- []byte(frame)
}

func (c *fakeConn) Read(ctx context.Context) ([]byte, error) {
	c.mu.Lock()
	c.reading++
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.reading--
		c.mu.Unlock()
	}()

	select {
	case frame := <-c.frames:
		return frame, nil
	case <-c.closed:
		return nil, errFakeConnClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *fakeConn) Write(_ context.Context, b []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, string(b))
	return nil
}

func (c *fakeConn) Close(string) error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *fakeConn) writtenCommands() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	commands := make([]string, 0, len(c.writes))
	for _, write := range c.writes {
		var frame struct {
			Command string `json:"command"`
			Data    string `json:"data"`
		}
		if err := json.Unmarshal([]byte(write), &frame); err != nil {
			continue
		}
		if frame.Data != "" {
			commands = append(commands, frame.Command+":"+frame.Data)
			continue
		}
		commands = append(commands, frame.Command)
	}
	return commands
}

func (c *fakeConn) readersActive() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reading
}

type recordingHandler struct {
	mu       sync.Mutex
	events   []Event
	controls []Control
	statuses []string
}

func (h *recordingHandler) OnEvent(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, ev)
}

func (h *recordingHandler) OnControl(c Control) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.controls = append(h.controls, c)
}

func (h *recordingHandler) OnStatus(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.statuses = append(h.statuses, msg)
}

func (h *recordingHandler) eventCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.events)
}

type sessionRun struct {
	result SessionResult
	err    error
}

func runSessionAsync(ctx context.Context, conn Conn, opts SessionOptions) <-chan sessionRun {
	done := make(chan sessionRun, 1)
	go func() {
		result, err := RunSession(ctx, conn, opts)
		done <- sessionRun{result: result, err: err}
	}()
	return done
}

func awaitSession(t *testing.T, done <-chan sessionRun) sessionRun {
	t.Helper()
	select {
	case run := <-done:
		return run
	case <-time.After(2 * time.Second):
		t.Fatal("RunSession did not return")
		return sessionRun{}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func testSessionOptions(handler Handler) SessionOptions {
	return SessionOptions{
		Identifier:  NewIdentifier("blog", nil),
		PingTimeout: 500 * time.Millisecond,
		Now:         func() time.Time { return time.Unix(1_700_000_000, 0) },
		Handler:     handler,
	}
}

func TestRunSessionSubscribesRenewsAndUnsubscribes(t *testing.T) {
	restore := minRenewDelay
	minRenewDelay = testTick
	t.Cleanup(func() { minRenewDelay = restore })

	conn := newFakeConn(`{"type":"welcome"}`)
	handler := &recordingHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := runSessionAsync(ctx, conn, testSessionOptions(handler))

	waitFor(t, "subscribe frame", func() bool { return len(conn.writtenCommands()) == 1 })
	if got := conn.writtenCommands()[0]; got != "subscribe" {
		t.Fatalf("first command = %q, want subscribe", got)
	}

	conn.push(`{"type":"confirm_subscription","identifier":"x"}`)
	conn.push(`{"identifier":"x","message":{"type":"subscribed","subscription_id":"sub_1","seq_at_create":1,"resync":true,"lease":{"expires_at":1700000120,"renew_after":1699999990}}}`)

	waitFor(t, "renew frame", func() bool { return len(conn.writtenCommands()) == 2 })
	if got := conn.writtenCommands()[1]; got != `message:{"action":"renew"}` {
		t.Fatalf("second command = %q, want renew", got)
	}

	cancel()
	run := awaitSession(t, done)
	if run.result != ResultDone || run.err != nil {
		t.Fatalf("result = %v, err = %v, want ResultDone", run.result, run.err)
	}
	commands := conn.writtenCommands()
	if commands[len(commands)-1] != "unsubscribe" {
		t.Fatalf("commands = %v, want trailing unsubscribe", commands)
	}
	if conn.readersActive() != 0 {
		t.Fatal("reader goroutine still blocked in Read")
	}
}

func TestRunSessionRenewedInactive(t *testing.T) {
	conn := newFakeConn(
		`{"type":"welcome"}`,
		`{"type":"confirm_subscription","identifier":"x"}`,
		`{"identifier":"x","message":{"type":"renewed","results":[{"status":"inactive","resync":true}]}}`,
	)
	run := awaitSession(t, runSessionAsync(context.Background(), conn, testSessionOptions(&recordingHandler{})))
	if run.result != ResultInactive || run.err != nil {
		t.Fatalf("result = %v, err = %v, want ResultInactive", run.result, run.err)
	}
}

func TestRunSessionSubscriptionError(t *testing.T) {
	conn := newFakeConn(
		`{"type":"welcome"}`,
		`{"identifier":"x","message":{"type":"subscription_error","code":"invalid_query"}}`,
	)
	run := awaitSession(t, runSessionAsync(context.Background(), conn, testSessionOptions(&recordingHandler{})))

	var subErr *SubscriptionError
	if !errors.As(run.err, &subErr) {
		t.Fatalf("err = %v, want *SubscriptionError", run.err)
	}
	if !subErr.Fatal() {
		t.Fatal("invalid_query must be fatal")
	}
}

func TestRunSessionDedupesEvents(t *testing.T) {
	event := `{"identifier":"x","message":{"event_id":"ev_1","event":"added","id":"1","type":"channel_entries.created"}}`
	conn := newFakeConn(`{"type":"welcome"}`, `{"type":"confirm_subscription","identifier":"x"}`, event, event)
	handler := &recordingHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	done := runSessionAsync(ctx, conn, testSessionOptions(handler))

	waitFor(t, "event delivery", func() bool { return handler.eventCount() == 1 })
	time.Sleep(20 * time.Millisecond)
	if got := handler.eventCount(); got != 1 {
		t.Fatalf("event count = %d, want 1", got)
	}
	cancel()
	awaitSession(t, done)
}

func TestRunSessionPingTimeout(t *testing.T) {
	conn := newFakeConn()
	opts := testSessionOptions(&recordingHandler{})
	opts.PingTimeout = 30 * time.Millisecond

	run := awaitSession(t, runSessionAsync(context.Background(), conn, opts))
	if run.result != ResultDisconnect || !errors.Is(run.err, ErrPingTimeout) {
		t.Fatalf("result = %v, err = %v, want ping timeout", run.result, run.err)
	}
	if conn.readersActive() != 0 {
		t.Fatal("reader goroutine still blocked in Read")
	}
}

func TestRunSessionPingResetsWatchdog(t *testing.T) {
	conn := newFakeConn(`{"type":"welcome"}`)
	opts := testSessionOptions(&recordingHandler{})
	opts.PingTimeout = 40 * time.Millisecond
	done := runSessionAsync(context.Background(), conn, opts)

	for range 3 {
		time.Sleep(15 * time.Millisecond)
		conn.push(`{"type":"ping","message":1700000000}`)
	}
	select {
	case run := <-done:
		t.Fatalf("session ended early: %v (%v)", run.result, run.err)
	default:
	}

	run := awaitSession(t, done)
	if run.result != ResultDisconnect {
		t.Fatalf("result = %v, want ResultDisconnect", run.result)
	}
}

func TestRunSessionDisconnectFrame(t *testing.T) {
	conn := newFakeConn(`{"type":"welcome"}`, `{"type":"disconnect","reason":"unauthorized","reconnect":false}`)
	run := awaitSession(t, runSessionAsync(context.Background(), conn, testSessionOptions(&recordingHandler{})))
	if run.result != ResultDisconnect {
		t.Fatalf("result = %v, want ResultDisconnect", run.result)
	}
	var rejected *GrantRejected
	if !errors.As(run.err, &rejected) {
		t.Fatalf("err = %v, want *GrantRejected", run.err)
	}

	conn = newFakeConn(`{"type":"welcome"}`, `{"type":"disconnect","reason":"server_restart"}`)
	run = awaitSession(t, runSessionAsync(context.Background(), conn, testSessionOptions(&recordingHandler{})))
	if run.result != ResultDisconnect || !strings.Contains(run.err.Error(), "server_restart") {
		t.Fatalf("result = %v, err = %v", run.result, run.err)
	}
}

func TestRunSessionSocketCloseDisconnects(t *testing.T) {
	conn := newFakeConn(`{"type":"welcome"}`)
	done := runSessionAsync(context.Background(), conn, testSessionOptions(&recordingHandler{}))
	waitFor(t, "subscribe frame", func() bool { return len(conn.writtenCommands()) == 1 })
	_ = conn.Close("")

	run := awaitSession(t, done)
	if run.result != ResultDisconnect || !errors.Is(run.err, errFakeConnClosed) {
		t.Fatalf("result = %v, err = %v", run.result, run.err)
	}
}
