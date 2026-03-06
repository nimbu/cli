package devproxy

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestWriteFilteredHeadersDropsHopByHop(t *testing.T) {
	dst := http.Header{}
	src := map[string]string{
		"Connection":        "keep-alive, X-Remove",
		"Trailer":           "Expires",
		"Transfer-Encoding": "chunked",
		"X-Remove":          "value",
		"X-Keep":            "ok",
		"Content-Type":      "text/html",
	}

	writeFilteredHeaders(dst, src)

	if got := dst.Get("Connection"); got != "" {
		t.Fatalf("connection header should be filtered, got %q", got)
	}
	if got := dst.Get("Trailer"); got != "" {
		t.Fatalf("trailer header should be filtered, got %q", got)
	}
	if got := dst.Get("Transfer-Encoding"); got != "" {
		t.Fatalf("transfer-encoding should be filtered, got %q", got)
	}
	if got := dst.Get("X-Remove"); got != "" {
		t.Fatalf("connection-token header should be filtered, got %q", got)
	}
	if got := dst.Get("X-Keep"); got != "ok" {
		t.Fatalf("expected x-keep header to remain, got %q", got)
	}
}

func TestWriteFilteredHeadersSplitsSetCookieLines(t *testing.T) {
	dst := http.Header{}
	src := map[string]string{
		"Set-Cookie": "a=1; Path=/\n b=2; Path=/",
	}

	writeFilteredHeaders(dst, src)

	values := dst.Values("Set-Cookie")
	if len(values) != 2 {
		t.Fatalf("expected 2 set-cookie values, got %d (%v)", len(values), values)
	}
	if values[0] != "a=1; Path=/" || values[1] != "b=2; Path=/" {
		t.Fatalf("unexpected cookies: %v", values)
	}
}

func TestWriteSimulatorResponseWritesDecodedTextBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	body := base64.StdEncoding.EncodeToString([]byte("hello world"))

	err := writeSimulatorResponse(recorder, &api.SimulatorResponse{
		Status:  http.StatusOK,
		Body:    body,
		Headers: map[string]string{"Content-Type": "text/html; charset=utf-8"},
	})
	if err != nil {
		t.Fatalf("writeSimulatorResponse: %v", err)
	}
	if recorder.Body.String() != "hello world" {
		t.Fatalf("body = %q, want hello world", recorder.Body.String())
	}
}
