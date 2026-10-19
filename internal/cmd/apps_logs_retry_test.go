package cmd

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

func TestAppsLogsTailRetriesTransientErrors(t *testing.T) {
	withFastAppsLogsPolling(t)

	baseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		switch {
		case call == 1:
			_, _ = w.Write([]byte(`[]`))
		case call <= 7:
			// Outlasts the HTTP transport's own retries so the tail loop sees it.
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"message":"unavailable"}`))
		case call == 8:
			_, _ = w.Write([]byte(`[{"id":"after","time":1751716801.5,"level":"INFO","data":"after the blip","context":null}]`))
		default:
			// The next poll proves the tail kept going; stop it there.
			cancel()
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()

	ctx, stdout, stderr := newAppsLogsTestContextFromBase(t, baseCtx, server.URL, output.Mode{})
	cmd := &AppsLogsCmd{App: "storefront", Tail: true, Since: "1751716800"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL}); err != nil {
		t.Fatalf("tail should ride out a transient error: %v", err)
	}
	if !strings.Contains(stdout.String(), "after the blip") {
		t.Fatalf("stdout = %q, want the log fetched after the blip", stdout.String())
	}
	if !strings.Contains(stderr.String(), "retrying in") {
		t.Fatalf("stderr = %q, want a retry warning", stderr.String())
	}
}

// A request that hits the client timeout matches context.DeadlineExceeded; it
// must be retried, not mistaken for Ctrl-C and end the tail with exit 0.
func TestAppsLogsTailRetriesRequestTimeout(t *testing.T) {
	withFastAppsLogsPolling(t)

	baseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			_, _ = w.Write([]byte(`[]`))
		case 2:
			select {
			case <-r.Context().Done():
			case <-time.After(time.Second):
			}
		case 3:
			_, _ = w.Write([]byte(`[{"id":"late","time":1751716801.5,"level":"INFO","data":"after the timeout","context":null}]`))
		default:
			cancel()
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()

	ctx, stdout, stderr := newAppsLogsTestContextFromBase(t, baseCtx, server.URL, output.Mode{})
	ctx.Value(rootFlagsKey{}).(*RootFlags).Timeout = 100 * time.Millisecond
	cmd := &AppsLogsCmd{App: "storefront", Tail: true, Since: "1751716800"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL}); err != nil {
		t.Fatalf("tail should retry a timed-out request: %v", err)
	}
	if !strings.Contains(stdout.String(), "after the timeout") {
		t.Fatalf("stdout = %q, want the log fetched after the timeout; stderr = %q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "retrying in") {
		t.Fatalf("stderr = %q, want a retry warning", stderr.String())
	}
}

func TestAppsLogsTailGivesUpAfterConsecutiveErrors(t *testing.T) {
	withFastAppsLogsPolling(t)
	original := appsLogsMaxConsecutiveErrors
	appsLogsMaxConsecutiveErrors = 3
	t.Cleanup(func() { appsLogsMaxConsecutiveErrors = original })

	baseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"message":"bad gateway"}`))
	}))
	defer server.Close()

	ctx, _, stderr := newAppsLogsTestContextFromBase(t, baseCtx, server.URL, output.Mode{})
	cmd := &AppsLogsCmd{App: "storefront", Tail: true, Since: "1751716800"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL})
	if baseCtx.Err() != nil {
		t.Fatal("tail kept retrying past the limit")
	}
	if err == nil || !strings.Contains(err.Error(), "3 consecutive failures") {
		t.Fatalf("err = %v, want give-up error after 3 consecutive failures", err)
	}
	if got := strings.Count(stderr.String(), "retrying in"); got != 2 {
		t.Fatalf("retry warnings = %d, want 2; stderr = %q", got, stderr.String())
	}
}

func TestAppsLogsTailFailsFastOnClientError(t *testing.T) {
	withFastAppsLogsPolling(t)

	baseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"app not found"}`))
	}))
	defer server.Close()

	ctx, _, stderr := newAppsLogsTestContextFromBase(t, baseCtx, server.URL, output.Mode{})
	cmd := &AppsLogsCmd{App: "storefront", Tail: true, Since: "1751716800"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL})
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || !apiErr.IsNotFound() {
		t.Fatalf("err = %v, want the 404 API error", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("log requests = %d, want 2 (no retry on 404)", calls.Load())
	}
	if strings.Contains(stderr.String(), "retrying") {
		t.Fatalf("stderr = %q, want no retry warning", stderr.String())
	}
}

// The error seen in production: an IPv6 route that briefly disappeared.
func TestTransientAppLogsErrorClassification(t *testing.T) {
	noRoute := &url.Error{Op: "Get", URL: "https://api.nimbu.io/apps/x/logs", Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.EHOSTUNREACH}}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"no route to host", noRoute, true},
		// How api.Client actually returns transport failures.
		{"no route to host from client", &api.Error{StatusCode: 0, Message: noRoute.Error(), Err: noRoute}, true},
		{"server error", &api.Error{StatusCode: http.StatusBadGateway}, true},
		{"rate limited", &api.Error{StatusCode: http.StatusTooManyRequests}, true},
		{"not found", &api.Error{StatusCode: http.StatusNotFound}, false},
		{"unauthorized", &api.Error{StatusCode: http.StatusUnauthorized}, false},
	}
	for _, tc := range cases {
		if got := transientAppLogsError(tc.err); got != tc.want {
			t.Errorf("%s: transient = %v, want %v", tc.name, got, tc.want)
		}
	}
}
