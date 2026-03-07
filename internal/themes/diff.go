package themes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

// DiffEntry reports one local-vs-remote mismatch.
type DiffEntry struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// DiffResult reports all detected liquid mismatches.
type DiffResult struct {
	Theme   string      `json:"theme"`
	Changes []DiffEntry `json:"changes,omitempty"`
	Entries []DiffEntry `json:"entries,omitempty"`
}

// RunDiff compares managed local liquid files with the remote theme.
func RunDiff(ctx context.Context, client *api.Client, cfg Config) (DiffResult, error) {
	remoteResources, err := FetchRemoteResources(ctx, client, cfg.Theme)
	if err != nil {
		return DiffResult{Theme: cfg.Theme}, err
	}

	scoped := make([]Resource, 0, len(remoteResources))
	for _, resource := range remoteResources {
		if resource.Kind == KindAsset {
			continue
		}
		if remoteInManagedScope(cfg, resource) {
			scoped = append(scoped, resource)
		}
	}

	filtered, err := FilterResources(cfg, scoped, Options{LiquidOnly: true})
	if err != nil {
		return DiffResult{Theme: cfg.Theme}, err
	}

	result := DiffResult{Theme: cfg.Theme}
	for _, resource := range filtered {
		projectPath, ok := ProjectPathForResource(cfg, resource)
		if !ok {
			continue
		}
		localPath := filepath.Join(cfg.ProjectRoot, filepath.FromSlash(projectPath))
		localData, err := os.ReadFile(localPath)
		if err != nil {
			if os.IsNotExist(err) {
				result.Changes = append(result.Changes, DiffEntry{Path: projectPath, Status: "missing"})
				continue
			}
			return result, err
		}
		remoteData, err := ReadContent(ctx, client, cfg.Theme, resource.Kind, resource.RemoteName)
		if err != nil {
			return result, fmt.Errorf("read %s: %w", resource.DisplayPath, err)
		}
		if normalizeDiffText(string(localData)) != normalizeDiffText(string(remoteData)) {
			result.Changes = append(result.Changes, DiffEntry{Path: projectPath, Status: "changed"})
		}
	}

	sort.SliceStable(result.Changes, func(i, j int) bool {
		return result.Changes[i].Path < result.Changes[j].Path
	})
	result.Entries = append([]DiffEntry{}, result.Changes...)
	return result, nil
}

func normalizeDiffText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}
