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
	if strings.HasPrefix(output, "\n") {
		t.Fatalf("did not expect spacer line for events-json output, got: %q", output)
	}
	if !strings.Contains(output, "GET /ons-werk (200)") {
		t.Fatalf("unexpected request log output: %q", output)
	}
}

func TestLoggingMiddlewarePrintsSingleSpacerBeforeFirstRequest(t *testing.T) {
	server := &Server{config: Config{UseColor: false}}
	handler := server.loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	output := captureStdout(t, func() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.com/first", nil))

		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.com/second", nil))
	})

	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected spacer plus two request logs, got %d lines: %q", len(lines), output)
	}
	if lines[0] != "" {
		t.Fatalf("expected first line to be blank spacer, got: %q", lines[0])
	}
	if !strings.Contains(lines[1], "GET /first (200)") {
		t.Fatalf("unexpected first request log: %q", lines[1])
	}
	if !strings.Contains(lines[2], "GET /second (200)") {
		t.Fatalf("unexpected second request log: %q", lines[2])
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

	// Read in a goroutine to avoid deadlock on Windows where pipe buffers are small.
	done := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- data
	}()

	fn()

	_ = w.Close()
	os.Stdout = original

	data := <-done
	_ = r.Close()
	return string(data)
}
