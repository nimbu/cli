package realtime

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Frame types sent by the ActionCable server.
const (
	FrameWelcome             = "welcome"
	FramePing                = "ping"
	FrameConfirmSubscription = "confirm_subscription"
	FrameRejectSubscription  = "reject_subscription"
	FrameDisconnect          = "disconnect"
)

// Control message types carried inside a frame's message payload.
const (
	ControlSubscribed       = "subscribed"
	ControlRenewed          = "renewed"
	ControlUnsubscribed     = "unsubscribed"
	ControlSubscriptionErr  = "subscription_error"
	ControlConfirmSubscribe = FrameConfirmSubscription
)

// Event names carried by realtime events.
const (
	EventAdded   = "added"
	EventChanged = "changed"
	EventRemoved = "removed"
	EventResync  = "resync"
)

// Event is a single live-query event envelope.
type Event struct {
	EventID    string          `json:"event_id"`
	Event      string          `json:"event"`
	Resource   string          `json:"resource"`
	ParentID   string          `json:"parent_id"`
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Object     json.RawMessage `json:"object,omitempty"`
	Changeset  json.RawMessage `json:"changeset,omitempty"`
	OccurredAt string          `json:"occurred_at"`
	Revision   int64           `json:"revision"`
	Raw        json.RawMessage `json:"-"`
}

// Lease describes when a subscription must be renewed, in unix seconds.
type Lease struct {
	ExpiresAt  float64
	RenewAfter float64
}

// Subscribed describes an accepted subscription and its lease.
type Subscribed struct {
	SubscriptionID string
	Resource       string
	Firehose       bool
	Query          map[string]string
	SeqAtCreate    int64
	Lease          Lease
	Resync         bool
	Raw            json.RawMessage
}

// Control is a non-event message from the realtime channel.
type Control struct {
	Type       string
	Code       string
	Subscribed *Subscribed
	Inactive   bool
	Raw        json.RawMessage
}

// Frame is one decoded ActionCable frame.
type Frame struct {
	Type       string          `json:"type"`
	Identifier string          `json:"identifier"`
	Message    json.RawMessage `json:"message"`
	Reason     string          `json:"reason"`
	Reconnect  *bool           `json:"reconnect"`
}

// ParseFrame decodes one ActionCable frame.
func ParseFrame(b []byte) (Frame, error) {
	var frame Frame
	if err := json.Unmarshal(b, &frame); err != nil {
		return Frame{}, fmt.Errorf("realtime: decode frame: %w", err)
	}
	return frame, nil
}

type subscribedPayload struct {
	SubscriptionID string          `json:"subscription_id"`
	Resource       string          `json:"resource"`
	Firehose       bool            `json:"firehose"`
	Query          json.RawMessage `json:"query"`
	SeqAtCreate    int64           `json:"seq_at_create"`
	Resync         bool            `json:"resync"`
	Status         string          `json:"status"`
	Lease          *struct {
		ExpiresAt  float64 `json:"expires_at"`
		RenewAfter float64 `json:"renew_after"`
	} `json:"lease"`
}

func (p subscribedPayload) toSubscribed(raw json.RawMessage) *Subscribed {
	sub := &Subscribed{
		SubscriptionID: p.SubscriptionID,
		Resource:       p.Resource,
		Firehose:       p.Firehose,
		SeqAtCreate:    p.SeqAtCreate,
		Resync:         p.Resync,
		Raw:            raw,
	}
	if len(p.Query) > 0 {
		query := map[string]string{}
		if err := json.Unmarshal(p.Query, &query); err == nil {
			sub.Query = query
		}
	}
	if p.Lease != nil {
		sub.Lease = Lease{ExpiresAt: p.Lease.ExpiresAt, RenewAfter: p.Lease.RenewAfter}
	}
	return sub
}

// ParseMessage decodes a frame message into either a control or an event.
func ParseMessage(raw json.RawMessage) (*Control, *Event, error) {
	if len(raw) == 0 {
		return nil, nil, errors.New("realtime: empty message")
	}

	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, nil, fmt.Errorf("realtime: decode message: %w", err)
	}

	switch probe.Type {
	case ControlSubscribed:
		var payload subscribedPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, nil, fmt.Errorf("realtime: decode subscribed: %w", err)
		}
		return &Control{Type: probe.Type, Subscribed: payload.toSubscribed(raw), Raw: raw}, nil, nil

	case ControlRenewed:
		control, err := parseRenewed(raw)
		if err != nil {
			return nil, nil, err
		}
		return control, nil, nil

	case ControlUnsubscribed:
		var payload subscribedPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, nil, fmt.Errorf("realtime: decode unsubscribed: %w", err)
		}
		return &Control{Type: probe.Type, Inactive: payload.Status == "inactive", Raw: raw}, nil, nil

	case ControlSubscriptionErr:
		var payload struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, nil, fmt.Errorf("realtime: decode subscription_error: %w", err)
		}
		return &Control{Type: probe.Type, Code: payload.Code, Raw: raw}, nil, nil
	}

	var event Event
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, nil, fmt.Errorf("realtime: decode event: %w", err)
	}
	if event.EventID == "" || event.Event == "" {
		return nil, nil, errors.New("realtime: message is neither a control nor an event")
	}
	event.Raw = raw
	return nil, &event, nil
}

func parseRenewed(raw json.RawMessage) (*Control, error) {
	var payload struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("realtime: decode renewed: %w", err)
	}

	control := &Control{Type: ControlRenewed, Raw: raw}
	for _, result := range payload.Results {
		var entry subscribedPayload
		if err := json.Unmarshal(result, &entry); err != nil {
			return nil, fmt.Errorf("realtime: decode renewed result: %w", err)
		}
		if entry.Status == "inactive" {
			control.Inactive = true
			continue
		}
		if control.Subscribed == nil {
			control.Subscribed = entry.toSubscribed(result)
		}
	}
	return control, nil
}
