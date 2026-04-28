package devproxy

import (
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

var templateDirs = []string{"layouts", "templates", "snippets"}

// CacheStatus exposes runtime cache metadata for diagnostics.
type CacheStatus struct {
	Dirty               bool   `json:"dirty"`
	FallbackScanEnabled bool   `json:"fallback_scan_enabled"`
	Fingerprint         string `json:"fingerprint"`
	LastBuildAt         string `json:"last_build_at,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	OverlayCount        int    `json:"overlay_count"`
	RebuildCount        uint64 `json:"rebuild_count"`
	StaleServedCount    uint64 `json:"stale_served_count"`
	WatchEnabled        bool   `json:"watch_enabled"`
	WatchError          string `json:"watch_error,omitempty"`
}

// TemplateCache caches compressed template payloads and invalidates on changes.
type TemplateCache struct {
	logger *Logger
	root   string

	maxScanEvery time.Duration
	watchEnabled bool

	watcher     *fsnotify.Watcher
	watchErr    string
	watchMu     sync.Mutex
	watchedDirs map[string]struct{}

	mu          sync.Mutex
	buildingCh  chan struct{}
	compressed  string
	dirty       bool
	fingerprint string
	lastBuildAt time.Time
	lastError   string
	lastStale   bool
	overlays    []TemplateOverlay

	hits             atomic.Uint64
	overlayVersion   uint64
	rebuilds         atomic.Uint64
	staleServed      atomic.Uint64
	scanInvalidation atomic.Uint64
	watchInvalid     atomic.Uint64

	stopCh chan struct{}
	wg     sync.WaitGroup

	stopErr  error
	stopOnce sync.Once
}

func NewTemplateCache(root string, watchEnabled bool, scanInterval time.Duration, logger *Logger) *TemplateCache {
	if scanInterval <= 0 {
		scanInterval = 3 * time.Second
	}

	return &TemplateCache{
		logger:       logger,
		root:         root,
		maxScanEvery: scanInterval,
		stopCh:       make(chan struct{}),
		watchEnabled: watchEnabled,
		watchedDirs:  map[string]struct{}{},
		dirty:        true,
	}
}

func (c *TemplateCache) Start() error {
	if _, _, err := c.rebuild(); err != nil {
		return err
	}

	if c.watchEnabled {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			c.setWatchErr(err.Error())
			c.logger.Warn("template watcher init failed; using fallback scan", map[string]any{"error": err.Error()})
		} else {
			c.watcher = watcher
			for _, dir := range templateDirs {
				if err := c.addWatchTree(filepath.Join(c.root, dir)); err != nil {
					c.logger.Warn("template watcher add failed", map[string]any{"dir": dir, "error": err.Error()})
				}
			}
			c.wg.Add(1)
			go c.watchLoop()
		}
	}

	c.wg.Add(1)
	go c.scanLoop()
	return nil
}

func (c *TemplateCache) Stop() error {
	c.stopOnce.Do(func() {
		close(c.stopCh)
		c.wg.Wait()

		if c.watcher != nil {
			c.stopErr = c.watcher.Close()
		}
	})
	return c.stopErr
}

// GetCompressed returns compressed template payload.
// stale=true indicates stale cache served due to rebuild error.
func (c *TemplateCache) GetCompressed() (code string, stale bool, err error) {
	c.mu.Lock()
	if !c.dirty && c.compressed != "" {
		c.hits.Add(1)
		code = c.compressed
		c.mu.Unlock()
		return code, false, nil
	}

	if c.buildingCh != nil {
		ch := c.buildingCh
		c.mu.Unlock()
		<-ch
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.compressed == "" && c.lastError != "" {
			return "", false, errors.New(c.lastError)
		}
		return c.compressed, c.lastStale, nil
	}

	c.buildingCh = make(chan struct{})
	c.lastStale = false
	previous := c.compressed
	c.mu.Unlock()

	code, stale, err = c.rebuild()

	c.mu.Lock()
	close(c.buildingCh)
	c.buildingCh = nil
	c.lastStale = stale
	if err != nil && previous != "" {
		c.staleServed.Add(1)
		c.lastError = err.Error()
		c.lastStale = true
		c.mu.Unlock()
		return previous, true, nil
	}
	c.mu.Unlock()
	return code, stale, err
}

func (c *TemplateCache) Status() CacheStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	status := CacheStatus{
		Dirty:               c.dirty,
		FallbackScanEnabled: c.maxScanEvery > 0,
		Fingerprint:         c.fingerprint,
		LastError:           c.lastError,
		OverlayCount:        len(c.overlays),
		RebuildCount:        c.rebuilds.Load(),
		StaleServedCount:    c.staleServed.Load(),
		WatchEnabled:        c.watchEnabled,
		WatchError:          c.watchErr,
	}
	if !c.lastBuildAt.IsZero() {
		status.LastBuildAt = c.lastBuildAt.UTC().Format(time.RFC3339Nano)
	}
	return status
}

func (c *TemplateCache) SetOverlays(overlays []TemplateOverlay) error {
	normalized, err := normalizedOverlays(overlays)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.overlays = append([]TemplateOverlay{}, normalized...)
	c.overlayVersion++
	c.dirty = true
	c.mu.Unlock()

	c.logger.Debug("template overlays updated", map[string]any{"count": len(normalized)})
	return nil
}

func (c *TemplateCache) overlaysSnapshot() ([]TemplateOverlay, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]TemplateOverlay{}, c.overlays...), c.overlayVersion
}

func (c *TemplateCache) markDirty(reason string) {
	c.mu.Lock()
	c.dirty = true
	c.mu.Unlock()

	if reason != "" {
		c.logger.Debug("template cache marked dirty", map[string]any{"reason": reason})
	}
}

func (c *TemplateCache) rebuild() (string, bool, error) {
	start := time.Now()
	for {
		overlays, version := c.overlaysSnapshot()
		compressed, fingerprint, err := buildCompressedTemplates(c.root, overlays)
		if err != nil {
			return "", false, err
		}
		if !c.commitRebuild(version, compressed, fingerprint) {
			continue
		}

		c.rebuilds.Add(1)
		c.logger.Debug("template cache rebuilt", map[string]any{
			"duration_ms": time.Since(start).Milliseconds(),
			"fingerprint": fingerprint,
		})

		return compressed, false, nil
	}
}

func (c *TemplateCache) commitRebuild(version uint64, compressed string, fingerprint string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if version != c.overlayVersion {
		c.dirty = true
		return false
	}
	c.compressed = compressed
	c.fingerprint = fingerprint
	c.dirty = false
	c.lastBuildAt = time.Now().UTC()
	c.lastError = ""
	return true
}

func (c *TemplateCache) watchLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		case event, ok := <-c.watcher.Events:
			if !ok {
				return
			}
			if err := c.handleWatchEvent(event); err != nil {
				c.logger.Warn("watch event handling failed", map[string]any{"error": err.Error(), "path": event.Name})
			}
		case err, ok := <-c.watcher.Errors:
			if !ok {
				return
			}
			c.setWatchErr(err.Error())
			c.logger.Warn("template watcher error", map[string]any{"error": err.Error()})
		}
	}
}

func (c *TemplateCache) handleWatchEvent(event fsnotify.Event) error {
	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename|fsnotify.Chmod) == 0 {
		return nil
	}

	if event.Op&fsnotify.Create != 0 {
		_ = c.addWatchTree(event.Name)
	}

	c.watchInvalid.Add(1)
	c.markDirty("watch_event")
	return nil
}

func (c *TemplateCache) scanLoop() {
	defer c.wg.Done()

	if c.maxScanEvery <= 0 {
		<-c.stopCh
		return
	}

	ticker := time.NewTicker(c.maxScanEvery)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			overlays, _ := c.overlaysSnapshot()
			fingerprint, err := computeTemplateFingerprint(c.root, overlays)
			if err != nil {
				c.logger.Warn("template fallback scan failed", map[string]any{"error": err.Error()})
				continue
			}

			c.mu.Lock()
			current := c.fingerprint
			isDirty := c.dirty
			c.mu.Unlock()

			if !isDirty && current != "" && current != fingerprint {
				c.scanInvalidation.Add(1)
				c.markDirty("scan_mismatch")
			}
		}
	}
}

func (c *TemplateCache) addWatchTree(root string) error {
	if c.watcher == nil {
		return nil
	}

	seen := map[string]struct{}{}
	return walkDirs(root, true, seen, func(dir string) error {
		c.watchMu.Lock()
		defer c.watchMu.Unlock()

		if _, ok := c.watchedDirs[dir]; ok {
			return nil
		}
		if err := c.watcher.Add(dir); err != nil {
			return err
		}
		c.watchedDirs[dir] = struct{}{}
		return nil
	})
}

func (c *TemplateCache) setWatchErr(value string) {
	c.mu.Lock()
	c.watchErr = value
	c.mu.Unlock()
}
