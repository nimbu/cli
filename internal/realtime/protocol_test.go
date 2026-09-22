package realtime

import (
	"encoding/json"
	"testing"
)

func TestIdentifierString(t *testing.T) {
	tests := []struct {
		name     string
		parentID string
		query    map[string]string
		want     string
	}{
		{
			name:     "without query",
			parentID: "blog",
			want:     `{"channel":"RealtimeApiChannel","resource":"channel_entries","parent_id":"blog"}`,
		},
		{
			name:     "empty query omitted",
			parentID: "blog",
			query:    map[string]string{},
			want:     `{"channel":"RealtimeApiChannel","resource":"channel_entries","parent_id":"blog"}`,
		},
		{
			name:     "query keys sorted",
			parentID: "blog",
			query:    map[string]string{"title.contains": "a", "_status": "published"},
			want:     `{"channel":"RealtimeApiChannel","resource":"channel_entries","parent_id":"blog","query":{"_status":"published","title.contains":"a"}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewIdentifier(tc.parentID, tc.query).String()
			if err != nil {
				t.Fatalf("String() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("String() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestParseFrame(t *testing.T) {
	frame, err := ParseFrame([]byte(`{"type":"disconnect","reason":"unauthorized","reconnect":false}`))
	if err != nil {
		t.Fatalf("ParseFrame error = %v", err)
	}
	if frame.Type != FrameDisconnect || frame.Reason != "unauthorized" {
		t.Fatalf("frame = %+v", frame)
	}
	if frame.Reconnect == nil || *frame.Reconnect {
		t.Fatalf("reconnect = %v, want false", frame.Reconnect)
	}

	if _, err := ParseFrame([]byte("not json")); err == nil {
		t.Fatal("ParseFrame(invalid) = nil error, want error")
	}
}

func TestParseMessageSubscribed(t *testing.T) {
	raw := json.RawMessage(`{"type":"subscribed","subscription_id":"sub_1","resource":"channel_entries","firehose":false,"query":{"_status":"published"},"seq_at_create":42,"resync":true,"lease":{"expires_at":1700000120.5,"renew_after":1700000060.25}}`)
	control, event, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("ParseMessage error = %v", err)
	}
	if event != nil {
		t.Fatalf("event = %+v, want nil", event)
	}
	if control.Type != ControlSubscribed || control.Subscribed == nil {
		t.Fatalf("control = %+v", control)
	}
	sub := control.Subscribed
	if sub.SubscriptionID != "sub_1" || sub.SeqAtCreate != 42 || !sub.Resync {
		t.Fatalf("subscribed = %+v", sub)
	}
	if sub.Lease.RenewAfter != 1700000060.25 || sub.Lease.ExpiresAt != 1700000120.5 {
		t.Fatalf("lease = %+v", sub.Lease)
	}
	if sub.Query["_status"] != "published" {
		t.Fatalf("query = %v", sub.Query)
	}
}

func TestParseMessageRenewed(t *testing.T) {
	control, _, err := ParseMessage(json.RawMessage(`{"type":"renewed","results":[{"subscription_id":"sub_1","lease":{"expires_at":10,"renew_after":5}}]}`))
	if err != nil {
		t.Fatalf("ParseMessage error = %v", err)
	}
	if control.Inactive {
		t.Fatal("Inactive = true, want false")
	}
	if control.Subscribed == nil || control.Subscribed.Lease.RenewAfter != 5 {
		t.Fatalf("subscribed = %+v", control.Subscribed)
	}

	control, _, err = ParseMessage(json.RawMessage(`{"type":"renewed","results":[{"status":"inactive","resync":true}]}`))
	if err != nil {
		t.Fatalf("ParseMessage error = %v", err)
	}
	if !control.Inactive {
		t.Fatal("Inactive = false, want true")
	}
}

func TestParseMessageUnsubscribedAndError(t *testing.T) {
	control, _, err := ParseMessage(json.RawMessage(`{"type":"unsubscribed","removed":true}`))
	if err != nil || control.Inactive {
		t.Fatalf("control = %+v, err = %v", control, err)
	}

	control, _, err = ParseMessage(json.RawMessage(`{"type":"unsubscribed","status":"inactive"}`))
	if err != nil || !control.Inactive {
		t.Fatalf("control = %+v, err = %v", control, err)
	}

	control, _, err = ParseMessage(json.RawMessage(`{"type":"subscription_error","code":"invalid_query"}`))
	if err != nil {
		t.Fatalf("ParseMessage error = %v", err)
	}
	if control.Code != "invalid_query" {
		t.Fatalf("code = %q", control.Code)
	}
}

func TestParseMessageEvent(t *testing.T) {
	raw := json.RawMessage(`{"event_id":"ev_1","event":"added","resource":"channel_entries","parent_id":"blog","id":"1","type":"channel_entries.created","object":{"title":"Hello"},"occurred_at":"2026-09-21T10:00:00Z","revision":7}`)
	control, event, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("ParseMessage error = %v", err)
	}
	if control != nil {
		t.Fatalf("control = %+v, want nil", control)
	}
	if event.EventID != "ev_1" || event.Event != EventAdded || event.Revision != 7 {
		t.Fatalf("event = %+v", event)
	}
	if string(event.Raw) != string(raw) {
		t.Fatalf("Raw = %s", event.Raw)
	}
}

func TestParseMessageRejectsUnknown(t *testing.T) {
	if _, _, err := ParseMessage(json.RawMessage(`{"hello":"world"}`)); err == nil {
		t.Fatal("ParseMessage(unknown) = nil error, want error")
	}
	if _, _, err := ParseMessage(nil); err == nil {
		t.Fatal("ParseMessage(nil) = nil error, want error")
	}
}

func TestDeduper(t *testing.T) {
	d := NewDeduper(2)
	if d.Seen("a") {
		t.Fatal("first Seen(a) = true")
	}
	if !d.Seen("a") {
		t.Fatal("second Seen(a) = false")
	}
	for range 2 {
		if d.Seen("") {
			t.Fatal("empty id must never be deduped")
		}
	}
	d.Seen("b")
	d.Seen("c")
	if d.Seen("a") {
		t.Fatal("a should have been evicted")
	}
}
