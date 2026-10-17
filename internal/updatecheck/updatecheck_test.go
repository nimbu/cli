package updatecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckerFetchesAndCachesLatestReleaseWhenCacheMissing(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("ETag", `"etag-1"`)
		_, _ = w.Write([]byte(`{
			"tag_name":"v0.1.2",
			"html_url":"https://github.com/nimbu/cli/releases/tag/v0.1.2",
			"draft":false,
			"prerelease":false
		}`))
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	stderr := &bytes.Buffer{}
	ctx := context.Background()

	c := checker{
		client:         srv.Client(),
		now:            fixedNow(),
		cachePath:      func() (string, error) { return cachePath, nil },
		executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
		latestURL:      srv.URL,
		requestTimeout: time.Second,
	}

	c.maybeNotify(ctx, stderr, "v0.1.1", Style{})

	if requests.Load() != 1 {
		t.Fatalf("expected 1 request, got %d", requests.Load())
	}

	if got := stderr.String(); !strings.Contains(got, "A new nimbu release is available: v0.1.2") {
		t.Fatalf("expected release notice, got %q", got)
	}
	if !strings.Contains(stderr.String(), "go install github.com/nimbu/cli/cmd/nimbu-cli@latest") {
		t.Fatalf("expected generic install hint, got %q", stderr.String())
	}

	cache := readCacheFile(t, cachePath)
	if !cache.UpdateAvailable {
		t.Fatal("expected cached update_available=true")
	}
	if cache.LatestVersion != "v0.1.2" {
		t.Fatalf("expected cached latest version, got %q", cache.LatestVersion)
	}
	if cache.LatestURL != "https://github.com/nimbu/cli/releases/tag/v0.1.2" {
		t.Fatalf("expected cached latest url, got %q", cache.LatestURL)
	}
	if cache.ETag != `"etag-1"` {
		t.Fatalf("expected cached etag, got %q", cache.ETag)
	}
}

func TestCheckerSkipsRefreshWhenCacheFresh(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	writeCacheFile(t, cachePath, cacheState{
		CheckedAt:       now.Add(-1 * time.Hour),
		LatestVersion:   "v0.1.2",
		LatestURL:       "https://github.com/nimbu/cli/releases/tag/v0.1.2",
		UpdateAvailable: true,
		ETag:            `"etag-1"`,
	})

	stderr := &bytes.Buffer{}
	ctx := context.Background()

	c := checker{
		client:         srv.Client(),
		now:            func() time.Time { return now },
		cachePath:      func() (string, error) { return cachePath, nil },
		executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
		latestURL:      srv.URL,
		requestTimeout: time.Second,
	}

	c.maybeNotify(ctx, stderr, "v0.1.1", Style{})

	if requests.Load() != 0 {
		t.Fatalf("expected no request, got %d", requests.Load())
	}
	if got := stderr.String(); !strings.Contains(got, "v0.1.2") {
		t.Fatalf("expected cached notice, got %q", got)
	}
}

func TestCheckerRefreshesCheckedAtOnNotModified(t *testing.T) {
	t.Parallel()

	var ifNoneMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ifNoneMatch = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `"etag-1"`)
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	oldCheckedAt := time.Date(2026, 3, 18, 8, 0, 0, 0, time.UTC)
	now := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	writeCacheFile(t, cachePath, cacheState{
		CheckedAt:       oldCheckedAt,
		LatestVersion:   "v0.1.2",
		LatestURL:       "https://github.com/nimbu/cli/releases/tag/v0.1.2",
		UpdateAvailable: true,
		ETag:            `"etag-1"`,
	})

	stderr := &bytes.Buffer{}
	ctx := context.Background()

	c := checker{
		client:         srv.Client(),
		now:            func() time.Time { return now },
		cachePath:      func() (string, error) { return cachePath, nil },
		executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
		latestURL:      srv.URL,
		requestTimeout: time.Second,
	}

	c.maybeNotify(ctx, stderr, "v0.1.1", Style{})

	if ifNoneMatch != `"etag-1"` {
		t.Fatalf("expected If-None-Match header, got %q", ifNoneMatch)
	}

	cache := readCacheFile(t, cachePath)
	if !cache.CheckedAt.Equal(now) {
		t.Fatalf("expected checked_at to update, got %s", cache.CheckedAt)
	}
	if cache.LatestVersion != "v0.1.2" {
		t.Fatalf("expected latest version to be preserved, got %q", cache.LatestVersion)
	}
	if !strings.Contains(stderr.String(), "v0.1.2") {
		t.Fatalf("expected cached notice, got %q", stderr.String())
	}
}

func TestCheckerMarksNoUpdateWhenLatestMatchesCurrent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"tag_name":"v0.1.1",
			"html_url":"https://github.com/nimbu/cli/releases/tag/v0.1.1",
			"draft":false,
			"prerelease":false
		}`))
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	stderr := &bytes.Buffer{}
	ctx := context.Background()

	c := checker{
		client:         srv.Client(),
		now:            fixedNow(),
		cachePath:      func() (string, error) { return cachePath, nil },
		executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
		latestURL:      srv.URL,
		requestTimeout: time.Second,
	}

	c.maybeNotify(ctx, stderr, "v0.1.1", Style{})

	if stderr.Len() != 0 {
		t.Fatalf("expected no notice, got %q", stderr.String())
	}
	cache := readCacheFile(t, cachePath)
	if cache.UpdateAvailable {
		t.Fatal("expected cached update_available=false")
	}
}

func TestCheckerIgnoresDraftAndPrereleaseReleases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "draft",
			body: `{"tag_name":"v0.1.2","html_url":"https://github.com/nimbu/cli/releases/tag/v0.1.2","draft":true,"prerelease":false}`,
		},
		{
			name: "prerelease",
			body: `{"tag_name":"v0.1.2","html_url":"https://github.com/nimbu/cli/releases/tag/v0.1.2","draft":false,"prerelease":true}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			cachePath := filepath.Join(t.TempDir(), "update-check.json")
			stderr := &bytes.Buffer{}
			ctx := context.Background()

			c := checker{
				client:         srv.Client(),
				now:            fixedNow(),
				cachePath:      func() (string, error) { return cachePath, nil },
				executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
				latestURL:      srv.URL,
				requestTimeout: time.Second,
			}

			c.maybeNotify(ctx, stderr, "v0.1.1", Style{})

			if stderr.Len() != 0 {
				t.Fatalf("expected no notice, got %q", stderr.String())
			}
			cache := readCacheFile(t, cachePath)
			if cache.UpdateAvailable {
				t.Fatal("expected cached update_available=false")
			}
		})
	}
}

func TestCheckerSkipsInvalidCurrentVersion(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	stderr := &bytes.Buffer{}
	ctx := context.Background()

	c := checker{
		client:         srv.Client(),
		now:            fixedNow(),
		cachePath:      func() (string, error) { return cachePath, nil },
		executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
		latestURL:      srv.URL,
		requestTimeout: time.Second,
	}

	c.maybeNotify(ctx, stderr, "dev", Style{})

	if requests.Load() != 0 {
		t.Fatalf("expected no request, got %d", requests.Load())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no notice, got %q", stderr.String())
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("expected no cache file, got %v", err)
	}
}

func TestCheckerFailsOpenOnTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.2","html_url":"https://github.com/nimbu/cli/releases/tag/v0.1.2","draft":false,"prerelease":false}`))
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	stderr := &bytes.Buffer{}
	ctx := context.Background()

	c := checker{
		client:         &http.Client{},
		now:            fixedNow(),
		cachePath:      func() (string, error) { return cachePath, nil },
		executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
		latestURL:      srv.URL,
		requestTimeout: 10 * time.Millisecond,
	}

	c.maybeNotify(ctx, stderr, "v0.1.1", Style{})

	if stderr.Len() != 0 {
		t.Fatalf("expected no notice, got %q", stderr.String())
	}
	// The timed-out attempt is recorded as a backoff, not as a result.
	cache := readCacheFile(t, cachePath)
	if cache.LatestVersion != "" || !cache.CheckedAt.IsZero() {
		t.Fatalf("expected no cached release after a timeout, got %+v", cache)
	}
	if !cache.RetryAfter.After(fixedNow()()) {
		t.Fatalf("expected a retry backoff after a timeout, got %+v", cache)
	}
}

func TestCheckerUsesHomebrewAndGenericUpgradeHints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		execPath string
		want     string
	}{
		{
			name:     "homebrew",
			execPath: "/opt/homebrew/Cellar/nimbu/0.1.1/bin/nimbu",
			want:     "brew upgrade nimbu/tap/nimbu",
		},
		{
			name:     "generic",
			execPath: "/usr/local/bin/nimbu",
			want:     "go install github.com/nimbu/cli/cmd/nimbu-cli@latest",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cachePath := filepath.Join(t.TempDir(), "update-check.json")
			now := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
			writeCacheFile(t, cachePath, cacheState{
				CheckedAt:       now.Add(-1 * time.Hour),
				LatestVersion:   "v0.1.2",
				LatestURL:       "https://github.com/nimbu/cli/releases/tag/v0.1.2",
				UpdateAvailable: true,
			})

			stderr := &bytes.Buffer{}
			ctx := context.Background()

			c := checker{
				client:         &http.Client{},
				now:            func() time.Time { return now },
				cachePath:      func() (string, error) { return cachePath, nil },
				executable:     func() (string, error) { return tc.execPath, nil },
				latestURL:      "http://example.invalid/releases/latest",
				requestTimeout: time.Second,
			}

			c.maybeNotify(ctx, stderr, "v0.1.1", Style{})

			if !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("expected notice to contain %q, got %q", tc.want, stderr.String())
			}
		})
	}
}

func TestCheckerSuppressesStaleCachedNoticeAfterLocalUpgrade(t *testing.T) {
	t.Parallel()

	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	writeCacheFile(t, cachePath, cacheState{
		CheckedAt:       now.Add(-1 * time.Hour),
		LatestVersion:   "v0.1.2",
		LatestURL:       "https://github.com/nimbu/cli/releases/tag/v0.1.2",
		UpdateAvailable: true,
	})

	stderr := &bytes.Buffer{}
	c := checker{
		client:         &http.Client{},
		now:            func() time.Time { return now },
		cachePath:      func() (string, error) { return cachePath, nil },
		executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
		latestURL:      "http://example.invalid/releases/latest",
		requestTimeout: time.Second,
	}

	c.maybeNotify(context.Background(), stderr, "v0.1.2", Style{})

	if stderr.Len() != 0 {
		t.Fatalf("expected no stale notice after local upgrade, got %q", stderr.String())
	}
	cache := readCacheFile(t, cachePath)
	if cache.UpdateAvailable {
		t.Fatal("expected cached update_available=false after local upgrade")
	}
}

func TestCheckerSuppressesNoticeForGitDescribeBuildAtSameRelease(t *testing.T) {
	t.Parallel()

	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	writeCacheFile(t, cachePath, cacheState{
		CheckedAt:       now.Add(-1 * time.Hour),
		LatestVersion:   "v0.1.2",
		LatestURL:       "https://github.com/nimbu/cli/releases/tag/v0.1.2",
		UpdateAvailable: true,
	})

	stderr := &bytes.Buffer{}
	c := checker{
		client:         &http.Client{},
		now:            func() time.Time { return now },
		cachePath:      func() (string, error) { return cachePath, nil },
		executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
		latestURL:      "http://example.invalid/releases/latest",
		requestTimeout: time.Second,
	}

	c.maybeNotify(context.Background(), stderr, "v0.1.2-1-geff58e1-dirty", Style{})

	if stderr.Len() != 0 {
		t.Fatalf("expected no notice for git describe build at current release, got %q", stderr.String())
	}
	cache := readCacheFile(t, cachePath)
	if cache.UpdateAvailable {
		t.Fatal("expected cached update_available=false after git describe current release")
	}
}

func TestBuildNoticeAppliesRequestedStyling(t *testing.T) {
	t.Parallel()

	got := buildNotice(cacheState{
		LatestVersion:   "v0.1.2",
		LatestURL:       "https://github.com/nimbu/cli/releases/tag/v0.1.2",
		UpdateAvailable: true,
	}, "v0.1.1", "/usr/local/bin/nimbu", Style{
		Bold: func(s string) string { return "<b>" + s + "</b>" },
		Dim:  func(s string) string { return "<d>" + s + "</d>" },
	})

	want := strings.Join([]string{
		"<b>A new nimbu release is available: v0.1.2</b>",
		"<d>To update, run: go install github.com/nimbu/cli/cmd/nimbu-cli@latest</d>",
		"<d>Release notes: https://github.com/nimbu/cli/releases/tag/v0.1.2</d>",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("unexpected notice:\n%s", got)
	}
}

func fixedNow() func() time.Time {
	return func() time.Time {
		return time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	}
}

func readCacheFile(t *testing.T, path string) cacheState {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}

	var cache cacheState
	if err := json.Unmarshal(data, &cache); err != nil {
		t.Fatalf("unmarshal cache: %v", err)
	}
	return cache
}

func writeCacheFile(t *testing.T, path string, cache cacheState) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}

	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
}

func TestCheckerDoesNotHoldExitForSlowRefresh(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.2","html_url":"https://github.com/nimbu/cli/releases/tag/v0.1.2","draft":false,"prerelease":false}`))
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	stderr := &bytes.Buffer{}
	c := checker{
		client:         srv.Client(),
		now:            fixedNow(),
		cachePath:      func() (string, error) { return cachePath, nil },
		executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
		latestURL:      srv.URL,
		requestTimeout: 10 * time.Second,
		notifyWait:     20 * time.Millisecond,
	}

	started := time.Now()
	pending := c.startRefresh(context.Background(), "v0.1.1")
	c.notifyAfter(context.Background(), pending, stderr, "v0.1.1", Style{})
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("notify blocked for %s on a slow refresh", elapsed)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no notice before the refresh finished, got %q", stderr.String())
	}

	// The refresh keeps running and still lands in the cache for next time.
	close(release)
	<-pending.done
	if cache := readCacheFile(t, cachePath); cache.LatestVersion != "v0.1.2" {
		t.Fatalf("expected background refresh to update the cache, got %+v", cache)
	}
}

func TestCheckerUsesPrefetchedRefresh(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.2","html_url":"https://github.com/nimbu/cli/releases/tag/v0.1.2","draft":false,"prerelease":false}`))
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	c := checker{
		client:         srv.Client(),
		now:            fixedNow(),
		cachePath:      func() (string, error) { return cachePath, nil },
		executable:     func() (string, error) { return "/usr/local/bin/nimbu", nil },
		latestURL:      srv.URL,
		requestTimeout: time.Second,
	}

	pending := c.startRefresh(context.Background(), "v0.1.1")
	if pending == nil {
		t.Fatal("expected a refresh for a missing cache")
	}
	stderr := &bytes.Buffer{}
	c.notifyAfter(context.Background(), pending, stderr, "v0.1.1", Style{})

	if requests.Load() != 1 {
		t.Fatalf("expected exactly one request, got %d", requests.Load())
	}
	if !strings.Contains(stderr.String(), "A new nimbu release is available: v0.1.2") {
		t.Fatalf("expected notice from the prefetched refresh, got %q", stderr.String())
	}
	if c.startRefresh(context.Background(), "v0.1.1") != nil {
		t.Fatal("expected no second refresh while the cache is fresh")
	}
}

func TestDisabledInCIAndByEnv(t *testing.T) {
	t.Parallel()

	cases := []struct {
		env  map[string]string
		want bool
	}{
		{env: map[string]string{}, want: false},
		{env: map[string]string{"CI": "true"}, want: true},
		{env: map[string]string{"CI": "1"}, want: true},
		{env: map[string]string{"CI": "false"}, want: false},
		{env: map[string]string{"CI": "0"}, want: false},
		{env: map[string]string{"CI": "no"}, want: false},
		{env: map[string]string{"CI": " False "}, want: false},
		{env: map[string]string{"NIMBU_NO_UPDATE_NOTIFIER": "1"}, want: true},
	}
	for _, tc := range cases {
		getenv := func(key string) string { return tc.env[key] }
		if got := disabled(getenv); got != tc.want {
			t.Fatalf("disabled(%v) = %v, want %v", tc.env, got, tc.want)
		}
	}
}

func TestIsHomebrewPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path   string
		prefix string
		want   bool
	}{
		{path: "/opt/homebrew/Cellar/nimbu/0.1.1/bin/nimbu", want: true},
		{path: "/opt/homebrew/bin/nimbu", want: true},
		{path: "/usr/local/Cellar/nimbu/0.1.1/bin/nimbu", want: true},
		{path: "/usr/local/opt/nimbu/bin/nimbu", want: true},
		{path: "/home/linuxbrew/.linuxbrew/bin/nimbu", want: true},
		{path: "/home/me/brew/Cellar/nimbu/1.0/bin/nimbu", want: true},
		{path: "/srv/brew/opt/nimbu/bin/nimbu", prefix: "/srv/brew", want: true},
		{path: "/usr/local/bin/nimbu", prefix: "/usr/local", want: false},
		{path: "/usr/local/bin/nimbu", want: false},
		{path: "/home/me/go/bin/nimbu-cli", want: false},
		{path: "", want: false},
	}
	for _, tc := range cases {
		if got := isHomebrewPath(tc.path, tc.prefix); got != tc.want {
			t.Fatalf("isHomebrewPath(%q, %q) = %v, want %v", tc.path, tc.prefix, got, tc.want)
		}
	}
}
