package themes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/nimbu/cli/internal/api"
)

// PullResult reports files written by a pull operation.
type PullResult struct {
	Theme   string   `json:"theme"`
	Written []Action `json:"written,omitempty"`
}

// PullOptions narrows pull scope.
type PullOptions = Options

// RunPull downloads managed remote files into the local project.
func RunPull(ctx context.Context, client *api.Client, cfg Config, opts Options) (PullResult, error) {
	remoteResources, err := FetchRemoteResources(ctx, client, cfg.Theme)
	if err != nil {
		return PullResult{Theme: cfg.Theme}, err
	}

	scoped := make([]Resource, 0, len(remoteResources))
	for _, resource := range remoteResources {
		if remoteInManagedScope(cfg, resource) {
			scoped = append(scoped, resource)
		}
	}
	filtered, err := FilterResources(cfg, scoped, opts)
	if err != nil {
		return PullResult{Theme: cfg.Theme}, err
	}

	result := PullResult{Theme: cfg.Theme}
	for _, resource := range filtered {
		projectPath, ok := ProjectPathForResource(cfg, resource)
		if !ok {
			continue
		}
		content, err := readResourceContent(ctx, client, cfg.Theme, resource)
		if err != nil {
			return result, fmt.Errorf("read %s: %w", resource.DisplayPath, err)
		}
		target := filepath.Join(cfg.ProjectRoot, filepath.FromSlash(projectPath))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return result, err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return result, err
		}
		result.Written = append(result.Written, Action{
			DisplayPath: resource.DisplayPath,
			Kind:        resource.Kind,
			LocalPath:   projectPath,
			RemoteName:  resource.RemoteName,
		})
	}
	sort.SliceStable(result.Written, func(i, j int) bool {
		return result.Written[i].LocalPath < result.Written[j].LocalPath
	})
	return result, nil
}
