package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/nimbu/cli/internal/config"
)

const completionCacheFileName = "completion-cache.json"

type completionCache struct {
	Entries map[string]completionCacheEntry `json:"entries"`
}

type completionCacheEntry struct {
	FetchedAt time.Time        `json:"fetched_at"`
	Items     []completionItem `json:"items"`
}

func (c *completionCache) set(key string, items []completionItem) {
	if c.Entries == nil {
		c.Entries = map[string]completionCacheEntry{}
	}
	c.Entries[key] = completionCacheEntry{FetchedAt: time.Now(), Items: items}
}

func readCompletionCache() completionCache {
	path, err := completionCachePath()
	if err != nil {
		return completionCache{Entries: map[string]completionCacheEntry{}}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return completionCache{Entries: map[string]completionCacheEntry{}}
	}
	var cache completionCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return completionCache{Entries: map[string]completionCacheEntry{}}
	}
	if cache.Entries == nil {
		cache.Entries = map[string]completionCacheEntry{}
	}
	return cache
}

func writeCompletionCache(cache completionCache) error {
	path, err := completionCachePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	merged := mergeCompletionCache(readCompletionCacheFile(path), cache)
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, completionCacheFileName+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func readCompletionCacheFile(path string) completionCache {
	data, err := os.ReadFile(path)
	if err != nil {
		return completionCache{Entries: map[string]completionCacheEntry{}}
	}
	var cache completionCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return completionCache{Entries: map[string]completionCacheEntry{}}
	}
	if cache.Entries == nil {
		cache.Entries = map[string]completionCacheEntry{}
	}
	return cache
}

func mergeCompletionCache(base completionCache, incoming completionCache) completionCache {
	return mergeCompletionCacheAt(base, incoming, time.Now())
}

func mergeCompletionCacheAt(base completionCache, incoming completionCache, now time.Time) completionCache {
	if base.Entries == nil {
		base.Entries = map[string]completionCacheEntry{}
	}
	for key, entry := range base.Entries {
		if now.Sub(entry.FetchedAt) > completionCacheTTL {
			delete(base.Entries, key)
		}
	}
	for key, entry := range incoming.Entries {
		base.Entries[key] = entry
	}
	return base
}

func completionCachePath() (string, error) {
	dir, err := config.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, completionCacheFileName), nil
}
