package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testGrant = "grant-super-secret-value"

type grantRequest struct {
	method string
	path   string
	body   string
	auth   string
	site   string
	accept string
}

func newGrantServer(t *testing.T, status int, payload string) (*httptest.Server, <-chan grantRequest) {
	t.Helper()
	seen := make(chan grantRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen <- grantRequest{
			method: r.Method,
			path:   r.URL.Path,
			body:   string(body),
			auth:   r.Header.Get("Authorization"),
			site:   r.Header.Get("X-Nimbu-Site"),
			accept: r.Header.Get("Accept"),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(server.Close)
	return server, seen
}

func TestMintRealtimeGrant(t *testing.T) {
	payload, err := json.Marshal(map[string]string{
		"grant":      testGrant,
		"expires_at": "2026-09-21T10:02:00Z",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	server, seen := newGrantServer(t, http.StatusCreated, string(payload))
	client := New(server.URL, "token-123").WithSite("demo")

	grant, err := MintRealtimeGrant(context.Background(), client)
	if err != nil {
		t.Fatalf("MintRealtimeGrant error = %v", err)
	}
	if grant.Grant != testGrant {
		t.Fatalf("grant = %q", grant.Grant)
	}
	want := time.Date(2026, 9, 21, 10, 2, 0, 0, time.UTC)
	if !grant.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", grant.ExpiresAt, want)
	}

	req := <-seen
	if req.method != http.MethodPost || req.path != "/realtime/grants" {
		t.Fatalf("request = %s %s", req.method, req.path)
	}
	if req.body != "{}" {
		t.Fatalf("body = %q, want {}", req.body)
	}
	if req.auth != "Bearer token-123" {
		t.Fatalf("authorization = %q", req.auth)
	}
	if req.site != "demo" {
		t.Fatalf("site header = %q, want demo", req.site)
	}
	if req.accept != "application/json" {
		t.Fatalf("accept = %q", req.accept)
	}
}

func TestMintRealtimeGrantRedactsDebugLog(t *testing.T) {
	server, _ := newGrantServer(t, http.StatusCreated, `{"grant":"`+testGrant+`","expires_at":"2026-09-21T10:02:00Z"}`)

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	client := New(server.URL, "token-123").WithSite("demo").WithDebug(true)
	if _, err := MintRealtimeGrant(context.Background(), client); err != nil {
		t.Fatalf("MintRealtimeGrant error = %v", err)
	}

	logged := logs.String()
	if strings.Contains(logged, testGrant) {
		t.Fatalf("debug log leaks the grant:\n%s", logged)
	}
	if !strings.Contains(logged, "<redacted>") {
		t.Fatalf("debug log missing the redaction marker:\n%s", logged)
	}
}

func TestDebugLogKeepsOrdinaryBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	client := New(server.URL, "token").WithDebug(true)
	var result map[string]any
	if err := client.Get(context.Background(), "/ping", &result); err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if !strings.Contains(logs.String(), `{\"ok\":true}`) && !strings.Contains(logs.String(), `{"ok":true}`) {
		t.Fatalf("debug log lost the body:\n%s", logs.String())
	}
}

func TestMintRealtimeGrantReturnsAPIError(t *testing.T) {
	server, _ := newGrantServer(t, http.StatusForbidden, `{"error":"realtime is not enabled for this site"}`)
	client := New(server.URL, "token").WithSite("demo")

	_, err := MintRealtimeGrant(context.Background(), client)
	if err == nil {
		t.Fatal("MintRealtimeGrant = nil error, want failure")
	}
	if !IsForbidden(err) {
		t.Fatalf("err = %v, want a 403", err)
	}
	if strings.Contains(err.Error(), testGrant) {
		t.Fatalf("error leaks a grant: %v", err)
	}
}

func TestMintRealtimeGrantIgnoresReadonly(t *testing.T) {
	server, _ := newGrantServer(t, http.StatusCreated, `{"grant":"`+testGrant+`","expires_at":"2026-09-21T10:00:00Z"}`)
	client := New(server.URL, "token").WithSite("demo").WithReadonly(true)

	grant, err := MintRealtimeGrant(context.Background(), client)
	if err != nil {
		t.Fatalf("readonly client should still mint a grant: %v", err)
	}
	if grant.Grant != testGrant {
		t.Fatalf("unexpected grant %q", grant.Grant)
	}
}
