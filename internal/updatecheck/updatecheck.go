package updatecheck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/nimbu/cli/internal/config"
)

const (
	cacheFileName     = "update-check.json"
	cacheTTL          = 24 * time.Hour
	defaultLatestURL  = "https://api.github.com/repos/nimbu/cli/releases/latest"
	defaultTimeout    = time.Second
	disableEnvVarName = "NIMBU_NO_UPDATE_NOTIFIER"
)

type cacheState struct {
	CheckedAt       time.Time `json:"checked_at"`
	LatestVersion   string    `json:"latest_version,omitempty"`
	LatestURL       string    `json:"latest_url,omitempty"`
	UpdateAvailable bool      `json:"update_available"`
	ETag            string    `json:"etag,omitempty"`
}

type checker struct {
	client         *http.Client
	now            func() time.Time
	cachePath      func() (string, error)
	executable     func() (string, error)
	latestURL      string
	requestTimeout time.Duration
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

// MaybeNotify checks whether a newer release is available and prints an update
// notice to stderr when a fresh cached result indicates the user should upgrade.
func MaybeNotify(ctx context.Context, stderr io.Writer, currentVersion string, style Style) {
	if os.Getenv(disableEnvVarName) != "" {
		return
	}

	defaultChecker().maybeNotify(ctx, stderr, currentVersion, style)
}

func defaultChecker() checker {
	return checker{
		client:         &http.Client{},
		now:            time.Now,
		cachePath:      defaultCachePath,
		executable:     os.Executable,
		latestURL:      defaultLatestURL,
		requestTimeout: defaultTimeout,
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
	currentVersion, ok := normalizeCurrentVersion(currentVersion)
	if !ok {
		return
	}

	cache := c.loadCache()
	now := c.now()
	if cache == nil || now.Sub(cache.CheckedAt) >= cacheTTL {
		cache = c.refresh(ctx, currentVersion, cache)
	}

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
		return nil
	}
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
	_ = os.WriteFile(path, data, 0o600)
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
	if strings.Contains(filepath.ToSlash(executablePath), "/Cellar/nimbu/") {
		return "brew upgrade nimbu/tap/nimbu"
	}
	return "go install github.com/nimbu/cli/cmd/nimbu-cli@latest"
}
