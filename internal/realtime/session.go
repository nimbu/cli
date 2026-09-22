package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

// DefaultPingTimeout is how long a session waits for any frame before it
// treats the socket as dead.
const DefaultPingTimeout = 30 * time.Second

// minRenewDelay keeps a renew from firing in a tight loop when the server
// hands out a lease that is already due. It is a variable for tests.
var minRenewDelay = time.Second

// unsubscribeGrace bounds the best-effort goodbye sent on cancellation.
var unsubscribeGrace = 200 * time.Millisecond

// SessionResult explains why a single socket lifetime ended.
type SessionResult int

const (
	// ResultInactive means the subscription lapsed and must be recreated.
	ResultInactive SessionResult = iota
	// ResultDisconnect means the socket dropped and may be retried.
	ResultDisconnect
	// ResultOnce means the caller asked to stop after the first event.
	ResultOnce
	// ResultDone means the context was cancelled.
	ResultDone
)

// String renders the result for logs.
func (r SessionResult) String() string {
	switch r {
	case ResultInactive:
		return "inactive"
	case ResultDisconnect:
		return "disconnect"
	case ResultOnce:
		return "once"
	case ResultDone:
		return "done"
	default:
		return "unknown"
	}
}

// Handler receives everything a session observes.
type Handler interface {
	OnEvent(ev Event)
	OnControl(c Control)
	OnStatus(msg string)
}

// SessionOptions configures one socket lifetime.
type SessionOptions struct {
	Identifier  Identifier
	PingTimeout time.Duration
	Now         func() time.Time
	Dedupe      *Deduper
	Handler     Handler
}

// SubscriptionError is a subscription_error control returned by the server.
type SubscriptionError struct {
	Code string
}

func (e *SubscriptionError) Error() string {
	return fmt.Sprintf("realtime: subscription error: %s", e.Code)
}

// Fatal reports whether retrying the same subscription is pointless.
func (e *SubscriptionError) Fatal() bool {
	return e.Code == "invalid_query"
}

// GrantRejected is returned when the server refuses the grant, which means a
// fresh grant must be minted before reconnecting.
type GrantRejected struct {
	Reason string
}

func (e *GrantRejected) Error() string {
	return fmt.Sprintf("realtime: connection refused: %s", e.Reason)
}

// ErrPingTimeout is returned when no frame arrives within the ping timeout.
var ErrPingTimeout = errors.New("realtime: no frames received before the ping timeout")

type nopHandler struct{}

func (nopHandler) OnEvent(Event)     {}
func (nopHandler) OnControl(Control) {}
func (nopHandler) OnStatus(string)   {}

type session struct {
	conn        Conn
	identifier  string
	handler     Handler
	dedupe      *Deduper
	now         func() time.Time
	pingTimeout time.Duration
	confirmed   bool
}

// RunSession drives one socket from welcome to teardown.
func RunSession(ctx context.Context, conn Conn, opts SessionOptions) (SessionResult, error) {
	identifier, err := opts.Identifier.String()
	if err != nil {
		return ResultDisconnect, err
	}

	s := &session{
		conn:        conn,
		identifier:  identifier,
		handler:     opts.Handler,
		dedupe:      opts.Dedupe,
		now:         opts.Now,
		pingTimeout: opts.PingTimeout,
	}
	if s.handler == nil {
		s.handler = nopHandler{}
	}
	if s.dedupe == nil {
		s.dedupe = NewDeduper(DefaultDedupeCapacity)
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.pingTimeout <= 0 {
		s.pingTimeout = DefaultPingTimeout
	}

	return s.run(ctx)
}

type readResult struct {
	data []byte
	err  error
}

func (s *session) run(ctx context.Context) (SessionResult, error) {
	readCtx, cancelRead := context.WithCancel(context.WithoutCancel(ctx))
	frames := make(chan readResult)
	readerDone := make(chan struct{})

	go func() {
		defer close(readerDone)
		for {
			data, err := s.conn.Read(readCtx)
			select {
			case frames <- readResult{data: data, err: err}:
			case <-readCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	defer func() {
		_ = s.conn.Close("")
		cancelRead()
		select {
		case <-readerDone:
		case <-time.After(unsubscribeGrace):
		}
	}()

	watchdog := time.NewTimer(s.pingTimeout)
	defer watchdog.Stop()
	renew := time.NewTimer(time.Hour)
	stopTimer(renew)
	defer renew.Stop()

	welcomed := false

	for {
		select {
		case <-ctx.Done():
			s.sendUnsubscribe()
			return ResultDone, nil

		case <-watchdog.C:
			return ResultDisconnect, ErrPingTimeout

		case <-renew.C:
			if err := s.sendRenew(ctx); err != nil {
				return ResultDisconnect, err
			}

		case read := <-frames:
			if read.err != nil {
				if ctx.Err() != nil {
					return ResultDone, nil
				}
				return ResultDisconnect, fmt.Errorf("realtime: read: %w", read.err)
			}
			resetTimer(watchdog, s.pingTimeout)

			frame, err := ParseFrame(read.data)
			if err != nil {
				return ResultDisconnect, err
			}

			if !welcomed {
				if frame.Type != FrameWelcome {
					if result, handled, err := s.handlePreWelcome(frame); handled {
						return result, err
					}
					continue
				}
				welcomed = true
				s.handler.OnStatus("connected")
				if err := s.sendSubscribe(ctx); err != nil {
					return ResultDisconnect, err
				}
				continue
			}

			result, done, err := s.handleFrame(ctx, frame, renew)
			if done {
				return result, err
			}
		}
	}
}

func (s *session) handlePreWelcome(frame Frame) (SessionResult, bool, error) {
	if frame.Type == FrameDisconnect {
		return ResultDisconnect, true, disconnectError(frame.Reason)
	}
	return 0, false, nil
}

func (s *session) handleFrame(ctx context.Context, frame Frame, renew *time.Timer) (SessionResult, bool, error) {
	switch frame.Type {
	case FramePing, FrameWelcome:
		return 0, false, nil

	case FrameConfirmSubscription:
		s.confirmed = true
		s.handler.OnControl(Control{Type: ControlConfirmSubscribe})
		s.handler.OnStatus("subscribed")
		return 0, false, nil

	case FrameRejectSubscription:
		return ResultDisconnect, true, errors.New("realtime: subscription rejected")

	case FrameDisconnect:
		return ResultDisconnect, true, disconnectError(frame.Reason)
	}

	if len(frame.Message) == 0 {
		return 0, false, nil
	}

	control, event, err := ParseMessage(frame.Message)
	if err != nil {
		return ResultDisconnect, true, err
	}
	if event != nil {
		if !s.dedupe.Seen(event.EventID) {
			s.handler.OnEvent(*event)
		}
		return 0, false, nil
	}

	return s.handleControl(ctx, *control, renew)
}

func (s *session) handleControl(ctx context.Context, control Control, renew *time.Timer) (SessionResult, bool, error) {
	s.handler.OnControl(control)

	switch control.Type {
	case ControlSubscribed:
		if control.Subscribed != nil {
			s.scheduleRenew(control.Subscribed.Lease, renew)
		}
		return 0, false, nil

	case ControlRenewed:
		if control.Inactive {
			return ResultInactive, true, nil
		}
		if control.Subscribed != nil {
			s.scheduleRenew(control.Subscribed.Lease, renew)
		}
		return 0, false, nil

	case ControlUnsubscribed:
		if control.Inactive {
			return ResultInactive, true, nil
		}
		if ctx.Err() != nil {
			return ResultDone, true, nil
		}
		return ResultDisconnect, true, errors.New("realtime: subscription removed by the server")

	case ControlSubscriptionErr:
		return ResultDisconnect, true, &SubscriptionError{Code: control.Code}
	}

	return 0, false, nil
}

func (s *session) scheduleRenew(lease Lease, renew *time.Timer) {
	if lease.RenewAfter <= 0 {
		stopTimer(renew)
		return
	}
	delay := unixFloat(lease.RenewAfter).Sub(s.now())
	if delay < minRenewDelay {
		delay = minRenewDelay
	}
	resetTimer(renew, delay)
}

func (s *session) sendSubscribe(ctx context.Context) error {
	return s.send(ctx, map[string]string{
		"command":    "subscribe",
		"identifier": s.identifier,
	})
}

func (s *session) sendRenew(ctx context.Context) error {
	return s.send(ctx, map[string]string{
		"command":    "message",
		"identifier": s.identifier,
		"data":       `{"action":"renew"}`,
	})
}

func (s *session) sendUnsubscribe() {
	if !s.confirmed {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), unsubscribeGrace)
	defer cancel()
	_ = s.send(ctx, map[string]string{
		"command":    "unsubscribe",
		"identifier": s.identifier,
	})
}

func (s *session) send(ctx context.Context, command map[string]string) error {
	data, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("realtime: encode %s: %w", command["command"], err)
	}
	if err := s.conn.Write(ctx, data); err != nil {
		return fmt.Errorf("realtime: send %s: %w", command["command"], err)
	}
	return nil
}

func disconnectError(reason string) error {
	if reason == "" {
		reason = "disconnected"
	}
	if reason == "unauthorized" {
		return &GrantRejected{Reason: reason}
	}
	return fmt.Errorf("realtime: server disconnected: %s", reason)
}

// unixFloat converts fractional unix seconds to a time.
func unixFloat(seconds float64) time.Time {
	whole, frac := math.Modf(seconds)
	return time.Unix(int64(whole), int64(frac*float64(time.Second)))
}

func stopTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}

func resetTimer(t *time.Timer, d time.Duration) {
	stopTimer(t)
	t.Reset(d)
}
