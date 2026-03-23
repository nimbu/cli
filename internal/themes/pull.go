package themes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PullResult reports files written by a pull operation.
type PullResult struct {
	Theme            string   `json:"theme"`
	Written          []Action `json:"written,omitempty"`
	TimelineRendered bool     `json:"-"`
}

// PullOptions narrows pull scope.
type PullOptions = Options

// RunPull downloads managed remote files into the local project.
func RunPull(ctx context.Context, client *api.Client, cfg Config, opts Options) (PullResult, error) {
	result := PullResult{Theme: cfg.Theme}

	remoteResources, err := FetchRemoteResources(ctx, client, cfg.Theme)
	if err != nil {
		return result, err
	}

	scoped := make([]Resource, 0, len(remoteResources))
	for _, resource := range remoteResources {
		if remoteInManagedScope(cfg, resource) {
			scoped = append(scoped, resource)
		}
	}
	filtered, err := FilterResources(cfg, scoped, opts)
	if err != nil {
		return result, err
	}

	// Sort in kind order and build categories for the timeline.
	ordered := SortByKindOrder(filtered)
	byKind := GroupByKind(ordered)
	var categories []output.SyncCategory
	for _, k := range KindOrder {
		if n := len(byKind[k]); n > 0 {
			categories = append(categories, output.SyncCategory{Label: k.Collection(), Count: n})
		}
	}

	tl := output.SyncTimelineFromContext(ctx)

	// Zero files.
	if len(ordered) == 0 {
		if tl != nil {
			tl.NothingToDo()
			result.TimelineRendered = true
		}
		return result, nil
	}

	// Start timeline.
	if tl != nil {
		tl.SetCategories(categories)
		tl.Header()
	}

	// Download loop (kind-ordered).
	catIdx := -1
	currentKind := Kind("")
	for _, resource := range ordered {
		projectPath, ok := ProjectPathForResource(cfg, resource)
		if !ok {
			continue
		}

		if resource.Kind != currentKind {
			if catIdx >= 0 && tl != nil {
				tl.CategoryDone(catIdx)
			}
			currentKind = resource.Kind
			catIdx++
			if tl != nil {
				tl.StartCategory(catIdx)
			}
		}
		if tl != nil {
			tl.SetActiveFile(resource.DisplayPath)
		}

		content, err := readResourceContent(ctx, client, cfg.Theme, resource)
		if err != nil {
			if tl != nil {
				tl.FileFailed(resource.DisplayPath, formatAPIError(err))
				tl.ErrorFooter()
				result.TimelineRendered = true
			}
			return result, fmt.Errorf("read %s: %w", resource.DisplayPath, err)
		}
		target := filepath.Join(cfg.ProjectRoot, filepath.FromSlash(projectPath))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			if tl != nil {
				tl.FileFailed(resource.DisplayPath, err.Error())
				tl.ErrorFooter()
				result.TimelineRendered = true
			}
			return result, err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			if tl != nil {
				tl.FileFailed(resource.DisplayPath, err.Error())
				tl.ErrorFooter()
				result.TimelineRendered = true
			}
			return result, err
		}

		if tl != nil {
			tl.FileUploaded()
		}
		result.Written = append(result.Written, Action{
			DisplayPath: resource.DisplayPath,
			Kind:        resource.Kind,
			LocalPath:   projectPath,
			RemoteName:  resource.RemoteName,
		})
	}
	if catIdx >= 0 && tl != nil {
		tl.CategoryDone(catIdx)
	}

	if tl != nil {
		tl.Footer()
		result.TimelineRendered = true
	}

	sort.SliceStable(result.Written, func(i, j int) bool {
		return result.Written[i].LocalPath < result.Written[j].LocalPath
	})
	return result, nil
}
