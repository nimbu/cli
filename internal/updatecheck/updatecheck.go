package updatecheck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/envutil"
)

const (
	cacheFileName     = "update-check.json"
	cacheTTL          = 24 * time.Hour
	defaultLatestURL  = "https://api.github.com/repos/nimbu/cli/releases/latest"
	defaultTimeout    = time.Second
	disableEnvVarName = "NIMBU_NO_UPDATE_NOTIFIER"

	// defaultNotifyWait bounds how long a finished command may wait for an
	// in-flight refresh before exiting. The refresh normally started with the
	// command (Prefetch) and is long done; if not, this run stays quiet and
	// the next run tries again.
	defaultNotifyWait = 150 * time.Millisecond

	// failureBackoff is how long a failed or unfinished refresh holds off the
	// next one, so an offline, firewalled or rate-limited user does not pay a
	// request and the exit wait on every run.
	failureBackoff = time.Hour
)

type cacheState struct {
	CheckedAt       time.Time `json:"checked_at"`
	LatestVersion   string    `json:"latest_version,omitempty"`
	LatestURL       string    `json:"latest_url,omitempty"`
	UpdateAvailable bool      `json:"update_available"`
	ETag            string    `json:"etag,omitempty"`
	// RetryAfter holds off refreshes after an attempt that failed or did not
	// finish. A successful refresh clears it.
	RetryAfter time.Time `json:"retry_after,omitzero"`
}

type checker struct {
	client         *http.Client
	now            func() time.Time
	cachePath      func() (string, error)
	executable     func() (string, error)
	latestURL      string
	requestTimeout time.Duration
	// notifyWait bounds the wait for a refresh in maybeNotify; zero waits for
	// the refresh to finish (it is still bounded by requestTimeout).
	notifyWait time.Duration
}

// pendingRefresh is a cache refresh running in the background.
type pendingRefresh struct {
	done   chan struct{}
	result *cacheState
}

var prefetched struct {
	mu      sync.Mutex
	pending *pendingRefresh
}

type latestReleaseResponse struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// Style controls optional terminal styling for the notice output.
type Style struct {
	Bold func(string) string
	Dim  func(string) string
}

var gitDescribeVersionPattern = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)-\d+-g[0-9a-f]+(?:-dirty)?$`)

// Prefetch starts refreshing a stale update cache in the background so the
// network round trip overlaps the command instead of delaying its exit. Call
// it before running the command; MaybeNotify picks up the result.
func Prefetch(ctx context.Context, currentVersion string) {
	if disabled(os.Getenv) {
		return
	}
	pending := defaultChecker().startRefresh(ctx, currentVersion)
	prefetched.mu.Lock()
	prefetched.pending = pending
	prefetched.mu.Unlock()
}

// MaybeNotify checks whether a newer release is available and prints an update
// notice to stderr when a fresh cached result indicates the user should upgrade.
// It never blocks for longer than a short bounded wait on a stale cache.
func MaybeNotify(ctx context.Context, stderr io.Writer, currentVersion string, style Style) {
	if disabled(os.Getenv) {
		return
	}

	prefetched.mu.Lock()
	pending := prefetched.pending
	prefetched.pending = nil
	prefetched.mu.Unlock()

	c := defaultChecker()
	if pending != nil {
		c.notifyAfter(ctx, pending, stderr, currentVersion, style)
		return
	}
	c.maybeNotify(ctx, stderr, currentVersion, style)
}

// disabled reports whether the notifier is off: NIMBU_NO_UPDATE_NOTIFIER is
// set, or the process runs in CI, where nobody reads the notice and the
// network call only slows builds down.
func disabled(getenv func(string) string) bool {
	if getenv(disableEnvVarName) != "" {
		return true
	}
	return envutil.IsCI(getenv)
}

func defaultChecker() checker {
	return checker{
		client:         &http.Client{},
		now:            time.Now,
		cachePath:      defaultCachePath,
		executable:     os.Executable,
		latestURL:      defaultLatestURL,
		requestTimeout: defaultTimeout,
		notifyWait:     defaultNotifyWait,
	}
}

func defaultCachePath() (string, error) {
	dir, err := config.EnsureDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cacheFileName), nil
}

func (c checker) maybeNotify(ctx context.Context, stderr io.Writer, currentVersion string, style Style) {
	c.notifyAfter(ctx, c.startRefresh(ctx, currentVersion), stderr, currentVersion, style)
}

// startRefresh refreshes a missing or stale cache on a background goroutine.
// It returns nil when the cache is fresh or the version is not comparable.
func (c checker) startRefresh(ctx context.Context, currentVersion string) *pendingRefresh {
	currentVersion, ok := normalizeCurrentVersion(currentVersion)
	if !ok {
		return nil
	}
	cache := c.loadCache()
	if cache != nil && (c.now().Sub(cache.CheckedAt) < cacheTTL || c.now().Before(cache.RetryAfter)) {
		return nil
	}
	pending := &pendingRefresh{done: make(chan struct{})}
	go func() {
		defer close(pending.done)
		pending.result = c.refresh(ctx, currentVersion, cache)
	}()
	return pending
}

// notifyAfter waits (bounded by notifyWait) for a pending refresh, then
// prints the notice if the cache says an upgrade is available.
func (c checker) notifyAfter(ctx context.Context, pending *pendingRefresh, stderr io.Writer, currentVersion string, style Style) {
	currentVersion, ok := normalizeCurrentVersion(currentVersion)
	if !ok {
		return
	}

	var refreshed *cacheState
	if pending != nil {
		var timeout <-chan time.Time
		if c.notifyWait > 0 {
			timer := time.NewTimer(c.notifyWait)
			defer timer.Stop()
			timeout = timer.C
		}
		var cancelled <-chan struct{}
		if ctx != nil {
			cancelled = ctx.Done()
		}
		select {
		case <-pending.done:
			refreshed = pending.result
		case <-timeout:
			return
		case <-cancelled:
			return
		}
	}

	cache := refreshed
	if cache == nil {
		cache = c.loadCache()
	}
	now := c.now()

	if cache == nil || !cache.UpdateAvailable || now.Sub(cache.CheckedAt) >= cacheTTL {
		return
	}
	if latestVersion, ok := normalizeVersion(cache.LatestVersion); !ok || semver.Compare(latestVersion, currentVersion) <= 0 {
		cache.UpdateAvailable = false
		c.storeCache(*cache)
		return
	}

	executablePath := c.resolvedExecutablePath()
	notice := buildNotice(*cache, currentVersion, executablePath, style)
	if notice == "" {
		return
	}

	if stderr == nil {
		return
	}
	_, _ = io.WriteString(stderr, notice)
}

func (c checker) loadCache() *cacheState {
	path, err := c.cachePath()
	if err != nil {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var cache cacheState
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	return &cache
}

func (c checker) refresh(ctx context.Context, currentVersion string, existing *cacheState) *cacheState {
	reqCtx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	if ctx != nil {
		reqCtx, cancel = context.WithTimeout(ctx, c.requestTimeout)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.latestURL, nil)
	if err != nil {
		return nil
	}
	// Record the attempt before the request: a refresh cut short by process
	// exit or Ctrl-C (a blackholed network outlives a fast command) counts as
	// failed. The root context is canceled on every exit, so a canceled
	// request cannot tell a user's interrupt from a hung network.
	c.recordAttempt(existing, c.now().Add(failureBackoff))
	req.Header.Set("Accept", "application/vnd.github+json")
	if existing != nil && existing.ETag != "" {
		req.Header.Set("If-None-Match", existing.ETag)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotModified:
		if existing == nil {
			return nil
		}
		next := *existing
		next.CheckedAt = c.now()
		next.RetryAfter = time.Time{}
		if etag := resp.Header.Get("ETag"); etag != "" {
			next.ETag = etag
		}
		c.storeCache(next)
		return &next
	case http.StatusOK:
		var release latestReleaseResponse
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return nil
		}

		next := cacheState{
			CheckedAt: c.now(),
			ETag:      resp.Header.Get("ETag"),
		}
		if !release.Draft && !release.Prerelease {
			if latestVersion, ok := normalizeVersion(release.TagName); ok {
				next.LatestVersion = latestVersion
				next.LatestURL = release.HTMLURL
				next.UpdateAvailable = semver.Compare(latestVersion, currentVersion) > 0
			}
		}

		c.storeCache(next)
		return &next
	default:
		if until := rateLimitReset(resp.Header, c.now()); !until.IsZero() {
			c.recordAttempt(existing, until)
		}
		return nil
	}
}

// recordAttempt stores the existing cache with refreshes held off until
// until. The cached release and ETag are kept, so an earlier result survives.
func (c checker) recordAttempt(existing *cacheState, until time.Time) {
	var next cacheState
	if existing != nil {
		next = *existing
	}
	next.RetryAfter = until
	c.storeCache(next)
}

// rateLimitReset returns when GitHub allows the next request, from
// Retry-After or an exhausted X-RateLimit-Reset, when that is later than the
// default backoff. It is capped at the cache TTL.
func rateLimitReset(header http.Header, now time.Time) time.Time {
	var until time.Time
	if seconds, err := strconv.Atoi(strings.TrimSpace(header.Get("Retry-After"))); err == nil && seconds > 0 {
		until = now.Add(time.Duration(seconds) * time.Second)
	} else if header.Get("X-RateLimit-Remaining") == "0" {
		if epoch, err := strconv.ParseInt(strings.TrimSpace(header.Get("X-RateLimit-Reset")), 10, 64); err == nil {
			until = time.Unix(epoch, 0)
		}
	}
	if !until.After(now.Add(failureBackoff)) {
		return time.Time{}
	}
	if limit := now.Add(cacheTTL); until.After(limit) {
		return limit
	}
	return until
}

func (c checker) storeCache(cache cacheState) {
	path, err := c.cachePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	// Write to a temp file and rename so a refresh cut short by process exit
	// never leaves a half-written cache behind.
	tmp, err := os.CreateTemp(filepath.Dir(path), cacheFileName+".*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		return
	}
	_ = os.Chmod(tmpName, 0o600)
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
	}
}

func (c checker) resolvedExecutablePath() string {
	path, err := c.executable()
	if err != nil {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}
	return path
}

func normalizeCurrentVersion(version string) (string, bool) {
	version = strings.TrimSpace(version)
	if version == "" {
		return "", false
	}
	if match := gitDescribeVersionPattern.FindStringSubmatch(version); len(match) == 2 {
		version = match[1]
	}
	return normalizeVersion(version)
}

func normalizeVersion(version string) (string, bool) {
	version = strings.TrimSpace(version)
	if version == "" {
		return "", false
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	version = semver.Canonical(version)
	if version == "" {
		return "", false
	}
	return version, true
}

func buildNotice(cache cacheState, currentVersion, executablePath string, style Style) string {
	if !cache.UpdateAvailable || cache.LatestVersion == "" || cache.LatestURL == "" {
		return ""
	}

	headline := "A new nimbu release is available: " + cache.LatestVersion
	if style.Bold != nil {
		headline = style.Bold(headline)
	}

	updateLine := "To update, run: " + upgradeHint(executablePath)
	releaseNotesLine := "Release notes: " + cache.LatestURL
	if style.Dim != nil {
		updateLine = style.Dim(updateLine)
		releaseNotesLine = style.Dim(releaseNotesLine)
	}

	var b strings.Builder
	b.WriteString(headline)
	b.WriteString("\n")
	b.WriteString(updateLine)
	b.WriteString("\n")
	b.WriteString(releaseNotesLine)
	b.WriteString("\n")
	return b.String()
}

func upgradeHint(executablePath string) string {
	if isHomebrewPath(executablePath, os.Getenv("HOMEBREW_PREFIX")) {
		return "brew upgrade nimbu/tap/nimbu"
	}
	return "go install github.com/nimbu/cli/cmd/nimbu-cli@latest"
}

// homebrewPrefixes are locations owned by Homebrew: the whole Apple silicon
// and Linuxbrew prefixes, and the keg/opt trees of the Intel macOS prefix
// (/usr/local itself is shared with manual installs). Detection never shells
// out to `brew --prefix`.
var homebrewPrefixes = []string{
	"/opt/homebrew/",
	"/home/linuxbrew/.linuxbrew/",
	"/usr/local/Cellar/",
	"/usr/local/opt/",
}

// isHomebrewPath reports whether the resolved executable lives in a Homebrew
// installation: any Cellar/nimbu keg, a default Homebrew prefix, or the
// Cellar/opt trees under $HOMEBREW_PREFIX.
func isHomebrewPath(executablePath, brewPrefix string) bool {
	path := filepath.ToSlash(executablePath)
	if path == "" {
		return false
	}
	if strings.Contains(path, "/Cellar/nimbu/") {
		return true
	}
	for _, prefix := range homebrewPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	brewPrefix = strings.TrimRight(filepath.ToSlash(strings.TrimSpace(brewPrefix)), "/")
	if brewPrefix == "" {
		return false
	}
	return strings.HasPrefix(path, brewPrefix+"/Cellar/") || strings.HasPrefix(path, brewPrefix+"/opt/")
}
