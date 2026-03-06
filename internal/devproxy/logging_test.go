package devproxy

import (
	"strings"
	"testing"
)

func TestRequestLogLineFormat(t *testing.T) {
	line := requestLogLine("GET", "/ons-werk", 200, false)

	parts := strings.Split(line, " ")
	if len(parts) != 4 {
		t.Fatalf("unexpected format: %q", line)
	}
	if !strings.HasSuffix(parts[0], "Z") {
		t.Fatalf("timestamp should be UTC RFC3339-like: %q", parts[0])
	}
	if parts[1] != "GET" {
		t.Fatalf("method mismatch: %q", parts[1])
	}
	if parts[2] != "/ons-werk" {
		t.Fatalf("path mismatch: %q", parts[2])
	}
	if parts[3] != "(200)" {
		t.Fatalf("status mismatch: %q", parts[3])
	}
}

func TestRequestLogLineColorizesByStatusClass(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantColor string
	}{
		{name: "success", status: 200, wantColor: "\x1b[32m"},
		{name: "redirect", status: 302, wantColor: "\x1b[32m"},
		{name: "client error", status: 404, wantColor: "\x1b[33m"},
		{name: "server error", status: 500, wantColor: "\x1b[31m"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := requestLogLine("GET", "/ons-werk", tc.status, true)
			if !strings.Contains(line, "\x1b[2m") {
				t.Fatalf("expected dim timestamp, got: %q", line)
			}
			if !strings.Contains(line, tc.wantColor) {
				t.Fatalf("expected status color %q, got: %q", tc.wantColor, line)
			}
		})
	}
}
