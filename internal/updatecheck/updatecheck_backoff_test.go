package updatecheck

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// backoffChecker returns a checker against srv whose clock reads *now.
func backoffChecker(t *testing.T, srv *httptest.Server, now *time.Time) (checker, string) {
	t.Helper()
	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	return checker{
		client:         srv.Client(),
		now:            func() time.Time { return *now },
		cachePath:      func() (string, error) { return cachePath, nil },
		executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
		latestURL:      srv.URL,
		requestTimeout: time.Second,
	}, cachePath
}

func runRefresh(c checker, ctx context.Context) {
	if pending := c.startRefresh(ctx, "v0.1.1"); pending != nil {
		<-pending.done
	}
}

func TestCheckerBacksOffAfterFailedRefresh(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.2","html_url":"https://github.com/nimbu/cli/releases/tag/v0.1.2"}`))
	}))
	defer srv.Close()

	now := fixedNow()()
	c, cachePath := backoffChecker(t, srv, &now)
	writeCacheFile(t, cachePath, cacheState{
		CheckedAt:     now.Add(-2 * cacheTTL),
		LatestVersion: "v0.1.2",
		LatestURL:     "https://github.com/nimbu/cli/releases/tag/v0.1.2",
		ETag:          `"etag-1"`,
	})

	runRefresh(c, context.Background())
	if requests.Load() != 1 {
		t.Fatalf("expected one request, got %d", requests.Load())
	}
	cache := readCacheFile(t, cachePath)
	if cache.ETag != `"etag-1"` || cache.LatestVersion != "v0.1.2" {
		t.Fatalf("expected the earlier result to survive a failure, got %+v", cache)
	}

	now = now.Add(failureBackoff / 2)
	if c.startRefresh(context.Background(), "v0.1.1") != nil {
		t.Fatal("expected no refresh during the backoff")
	}

	now = now.Add(failureBackoff)
	fail.Store(false)
	runRefresh(c, context.Background())
	if requests.Load() != 2 {
		t.Fatalf("expected the refresh to resume after the backoff, got %d requests", requests.Load())
	}
	if cache := readCacheFile(t, cachePath); !cache.RetryAfter.IsZero() || !cache.CheckedAt.Equal(now) {
		t.Fatalf("expected a successful refresh to clear the backoff, got %+v", cache)
	}
}

func TestCheckerBacksOffWhenNetworkFails(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // connection refused

	now := fixedNow()()
	c, cachePath := backoffChecker(t, srv, &now)
	runRefresh(c, context.Background())

	if cache := readCacheFile(t, cachePath); !cache.RetryAfter.Equal(now.Add(failureBackoff)) {
		t.Fatalf("expected a backoff after a network failure, got %+v", cache)
	}
	stderr := &bytes.Buffer{}
	started := time.Now()
	c.notifyWait = time.Second
	c.maybeNotify(context.Background(), stderr, "v0.1.1", Style{})
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("expected no exit wait during the backoff, waited %s", elapsed)
	}
}

func TestCheckerHonoursRateLimitReset(t *testing.T) {
	t.Parallel()

	now := fixedNow()()
	reset := now.Add(3 * time.Hour)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c, cachePath := backoffChecker(t, srv, &now)
	runRefresh(c, context.Background())

	if cache := readCacheFile(t, cachePath); !cache.RetryAfter.Equal(reset) {
		t.Fatalf("expected the backoff to last until the rate-limit reset %s, got %+v", reset, cache)
	}
}
