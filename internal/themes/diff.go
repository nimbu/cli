package themes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// DiffEntry reports one local-vs-remote mismatch.
type DiffEntry struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Diff   string `json:"diff,omitempty"`
}

// DiffOptions controls how much detail RunDiff collects.
type DiffOptions struct {
	// Content renders a unified diff (remote vs local) for each entry.
	Content bool
}

// DiffResult reports all detected liquid mismatches.
type DiffResult struct {
	Theme   string      `json:"theme"`
	Changes []DiffEntry `json:"changes,omitempty"`
	Entries []DiffEntry `json:"entries,omitempty"`
}

// RunDiff compares managed local liquid files with the remote theme.
func RunDiff(ctx context.Context, client *api.Client, cfg Config) (DiffResult, error) {
	return RunDiffWithOptions(ctx, client, cfg, DiffOptions{})
}

// RunDiffWithOptions compares managed local liquid files with the remote theme.
func RunDiffWithOptions(ctx context.Context, client *api.Client, cfg Config, opts DiffOptions) (DiffResult, error) {
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
				entry := DiffEntry{Path: projectPath, Status: "missing"}
				if opts.Content {
					remoteData, readErr := ReadContent(ctx, client, cfg.Theme, resource.Kind, resource.RemoteName)
					if readErr != nil {
						return result, fmt.Errorf("read %s: %w", resource.DisplayPath, readErr)
					}
					entry.Diff = unifiedDiff(projectPath, string(remoteData), "")
				}
				result.Changes = append(result.Changes, entry)
				continue
			}
			return result, err
		}
		remoteData, err := ReadContent(ctx, client, cfg.Theme, resource.Kind, resource.RemoteName)
		if err != nil {
			return result, fmt.Errorf("read %s: %w", resource.DisplayPath, err)
		}
		if normalizeDiffText(string(localData)) != normalizeDiffText(string(remoteData)) {
			entry := DiffEntry{Path: projectPath, Status: "changed"}
			if opts.Content {
				entry.Diff = unifiedDiff(projectPath, string(remoteData), string(localData))
			}
			result.Changes = append(result.Changes, entry)
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

// unifiedDiff renders a git-style unified diff between the remote and local
// version of a theme file. Line endings are normalized and a trailing newline
// is enforced so the diff matches the CLI's lenient change detection.
func unifiedDiff(path, remote, local string) string {
	return output.Unified("remote/"+path, "local/"+path, remote, local)
}
