package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// stubTokens is a TokenSource that renews to a fixed token.
type stubTokens struct {
	mu       sync.Mutex
	current  string
	next     string
	renewErr error
	renewals []string
}

func (s *stubTokens) Token(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current, nil
}

func (s *stubTokens) Renew(_ context.Context, rejected string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renewals = append(s.renewals, rejected)
	if s.renewErr != nil {
		return "", s.renewErr
	}
	s.current = s.next
	return s.current, nil
}

// authServer accepts only the bearer token "good" and echoes request bodies.
func authServer(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*seen = append(*seen, r.Header.Get("Authorization")+" "+string(body))
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"unauthorized"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClientRenewsRejectedTokenAndReplaysBody(t *testing.T) {
	var seen []string
	srv := authServer(t, &seen)
	tokens := &stubTokens{current: "stale", next: "good"}
	client := New(srv.URL, "").WithTokenSource(tokens).WithSite("demo")

	var out map[string]bool
	if err := client.Post(context.Background(), "/things", map[string]string{"a": "b"}, &out); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !out["ok"] {
		t.Fatalf("response = %v", out)
	}
	want := []string{`Bearer stale {"a":"b"}`, `Bearer good {"a":"b"}`}
	if len(seen) != 2 || seen[0] != want[0] || seen[1] != want[1] {
		t.Fatalf("requests = %q, want %q", seen, want)
	}
	if len(tokens.renewals) != 1 || tokens.renewals[0] != "stale" {
		t.Fatalf("renewals = %q", tokens.renewals)
	}
}

func TestClientRetriesAtMostOnce(t *testing.T) {
	var seen []string
	srv := authServer(t, &seen)
	tokens := &stubTokens{current: "stale", next: "still-bad"}

	err := New(srv.URL, "").WithTokenSource(tokens).Get(context.Background(), "/things", nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("error = %v, want 401", err)
	}
	if len(seen) != 2 {
		t.Fatalf("requests = %d, want 2", len(seen))
	}
}

func TestClientSurfacesRenewError(t *testing.T) {
	var seen []string
	srv := authServer(t, &seen)
	expired := errors.New("session expired")
	tokens := &stubTokens{current: "stale", renewErr: expired}

	resp, err := New(srv.URL, "").WithTokenSource(tokens).RawRequest(context.Background(), http.MethodGet, "/things", nil)
	if !errors.Is(err, expired) {
		t.Fatalf("error = %v, want the renew error", err)
	}
	if resp != nil {
		t.Fatal("response must be nil when renewal fails")
	}
}

func TestClientWithoutTokenSourceDoesNotRetry(t *testing.T) {
	var seen []string
	srv := authServer(t, &seen)

	err := New(srv.URL, "stale").Get(context.Background(), "/things", nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized || len(seen) != 1 {
		t.Fatalf("error = %v after %d requests, want one 401", err, len(seen))
	}
}

func TestDownloadURLRenewsRejectedToken(t *testing.T) {
	var seen []string
	srv := authServer(t, &seen)
	tokens := &stubTokens{current: "stale", next: "good"}

	resp, _, err := New(srv.URL, "").WithTokenSource(tokens).DownloadURL(context.Background(), "/file")
	if err != nil {
		t.Fatalf("DownloadURL: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(seen) != 2 {
		t.Fatalf("status = %d after %d requests", resp.StatusCode, len(seen))
	}
}
