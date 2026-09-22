package realtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type handshake struct {
	path      string
	grant     string
	protocols string
}

func TestDialNegotiatesSubprotocolAndSendsGrant(t *testing.T) {
	const grant = "grant-secret-123"
	seen := make(chan handshake, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- handshake{
			path:      r.URL.Path,
			grant:     r.URL.Query().Get("grant"),
			protocols: r.Header.Get("Sec-WebSocket-Protocol"),
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{Subprotocol}})
		if err != nil {
			return
		}
		defer conn.CloseNow() //nolint:errcheck // test server teardown
		if conn.Subprotocol() != Subprotocol {
			return
		}
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"welcome"}`))
		time.Sleep(20 * time.Millisecond)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := Dial(ctx, server.URL, grant, DialOptions{})
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer func() { _ = conn.Close("done") }()

	frame, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if string(frame) != `{"type":"welcome"}` {
		t.Fatalf("frame = %s", frame)
	}

	got := <-seen
	if got.path != "/ws" {
		t.Fatalf("path = %q, want /ws", got.path)
	}
	if got.grant != grant {
		t.Fatalf("grant = %q, want %q", got.grant, grant)
	}
	if !strings.Contains(got.protocols, Subprotocol) {
		t.Fatalf("subprotocol header = %q, want %q", got.protocols, Subprotocol)
	}
}

func TestDialErrorHidesGrant(t *testing.T) {
	const grant = "grant-secret-456"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	_, err := Dial(context.Background(), server.URL, grant, DialOptions{})
	if err == nil {
		t.Fatal("Dial = nil error, want failure")
	}
	if strings.Contains(err.Error(), grant) {
		t.Fatalf("error leaks the grant: %s", err)
	}
	if !strings.Contains(err.Error(), "/ws") {
		t.Fatalf("error = %q, want the dial target", err)
	}
}

func TestWebsocketURL(t *testing.T) {
	tests := []struct {
		host    string
		target  string
		display string
	}{
		{host: "example.nimbu.io", target: "wss://example.nimbu.io/ws?grant=g", display: "wss://example.nimbu.io/ws"},
		{host: "https://example.nimbu.io/", target: "wss://example.nimbu.io/ws?grant=g", display: "wss://example.nimbu.io/ws"},
		{host: "http://localhost:3000", target: "ws://localhost:3000/ws?grant=g", display: "ws://localhost:3000/ws"},
		{host: "ws://localhost:3000", target: "ws://localhost:3000/ws?grant=g", display: "ws://localhost:3000/ws"},
	}
	for _, tc := range tests {
		target, display, err := websocketURL(tc.host, "g")
		if err != nil {
			t.Fatalf("websocketURL(%q) error = %v", tc.host, err)
		}
		if target != tc.target || display != tc.display {
			t.Fatalf("websocketURL(%q) = %q, %q; want %q, %q", tc.host, target, display, tc.target, tc.display)
		}
	}

	if _, _, err := websocketURL("  ", "g"); err == nil {
		t.Fatal("websocketURL(empty) = nil error, want error")
	}
}
