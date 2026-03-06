package devproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestIsWebSocketUpgradeRequiresConnectionUpgrade(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "keep-alive")

	if isWebSocketUpgrade(req) {
		t.Fatal("expected false when connection header misses upgrade token")
	}

	req.Header.Set("Connection", "Upgrade")
	if !isWebSocketUpgrade(req) {
		t.Fatal("expected websocket upgrade detection to pass")
	}
}

func TestLoggingMiddlewareDisablesANSIForEventsJSON(t *testing.T) {
	server := &Server{config: Config{UseColor: true, EventsJSON: true}}
	handler := server.loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	output := captureStdout(t, func() {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/ons-werk", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	})

	if strings.Contains(output, "\x1b[") {
		t.Fatalf("expected plain request log, got ANSI: %q", output)
	}
	if !strings.Contains(output, "GET /ons-werk (200)") {
		t.Fatalf("unexpected request log output: %q", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}

	os.Stdout = w
	defer func() {
		os.Stdout = original
	}()

	fn()

	_ = w.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	_ = r.Close()
	return string(data)
}
