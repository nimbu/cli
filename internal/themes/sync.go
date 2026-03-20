package themes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// RunPush executes upload-only theme deployment.
func RunPush(ctx context.Context, client *api.Client, cfg Config, opts Options) (Result, error) {
	return run(ctx, client, cfg, opts, "push")
}

// RunSync executes delete-aware theme synchronization.
func RunSync(ctx context.Context, client *api.Client, cfg Config, opts Options) (Result, error) {
	return run(ctx, client, cfg, opts, "sync")
}

func run(ctx context.Context, client *api.Client, cfg Config, opts Options, mode string) (Result, error) {
	result := Result{DryRun: opts.DryRun, Mode: mode, Theme: cfg.Theme}
	if opts.Build {
		if err := RunBuild(ctx, cfg); err != nil {
			return result, err
		}
		result.Built = true
	}

	allLocal, err := CollectAll(cfg)
	if err != nil {
		return result, err
	}
	localByKey := mapByKey(allLocal)
	selection, err := compileSelectionFilter(cfg, opts)
	if err != nil {
		return result, err
	}

	uploads, deletes, err := planOperations(ctx, client, cfg, opts, mode, allLocal, localByKey, selection)
	if err != nil {
		return result, err
	}

	// Sort uploads in dependency order
	orderedUploads := SortByKindOrder(uploads)
	uploadsByKind := GroupByKind(orderedUploads)
	result.Uploaded = toActions(orderedUploads)
	result.Deleted = toActions(deletes)

	// Build category list for timeline
	var categories []output.SyncCategory
	for _, k := range KindOrder {
		if n := len(uploadsByKind[k]); n > 0 {
			categories = append(categories, output.SyncCategory{Label: k.Collection(), Count: n})
		}
	}

	tl := output.SyncTimelineFromContext(ctx)

	// Dry-run path
	if opts.DryRun {
		if tl != nil {
			tl.RenderPlan(categories, len(deletes))
			result.TimelineRendered = true
		}
		return result, nil
	}

	// Zero files
	if len(orderedUploads) == 0 && len(deletes) == 0 {
		if tl != nil {
			tl.NothingToDo()
			result.TimelineRendered = true
		}
		return result, nil
	}

	// Start timeline
	if tl != nil {
		tl.SetCategories(categories)
		tl.Header()
	}

	// Upload loop (dependency-ordered).
	// catIdx tracks position in the categories slice, which is built from the
	// same KindOrder iteration as orderedUploads — so kind transitions in the
	// sorted uploads always align 1:1 with category indices.
	catIdx := -1
	currentKind := Kind("")
	for _, resource := range orderedUploads {
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
		if err := Upsert(ctx, client, cfg.Theme, resource, opts.Force); err != nil {
			if tl != nil {
				tl.FileFailed(resource.DisplayPath, formatAPIError(err))
				tl.ErrorFooter()
				result.TimelineRendered = true
			}
			return result, fmt.Errorf("upload %s: %w", resource.DisplayPath, err)
		}
		if tl != nil {
			tl.FileUploaded()
		}
	}
	if catIdx >= 0 && tl != nil {
		tl.CategoryDone(catIdx)
	}

	// Delete loop
	if len(deletes) > 0 {
		if tl != nil {
			tl.StartDeletes(len(deletes))
		}
		for _, resource := range deletes {
			if tl != nil {
				tl.SetActiveDelete(resource.DisplayPath)
			}
			if err := Delete(ctx, client, cfg.Theme, resource); err != nil {
				if tl != nil {
					tl.DeleteFailed(resource.DisplayPath, formatAPIError(err))
					tl.ErrorFooter()
					result.TimelineRendered = true
				}
				return result, fmt.Errorf("delete %s: %w", resource.DisplayPath, err)
			}
			if tl != nil {
				tl.FileDeleted()
			}
		}
		if tl != nil {
			tl.DeletesDone()
		}
	}

	if tl != nil {
		tl.Footer()
		result.TimelineRendered = true
	}
	return result, nil
}

func formatAPIError(err error) string {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		status := http.StatusText(apiErr.StatusCode)
		if status != "" {
			return fmt.Sprintf("%d %s — %s", apiErr.StatusCode, status, apiErr.Message)
		}
		return apiErr.Message
	}
	return err.Error()
}

func planOperations(ctx context.Context, client *api.Client, cfg Config, opts Options, mode string, allLocal []Resource, localByKey map[resourceKey]Resource, selection *selectionFilter) ([]Resource, []Resource, error) {
	uploadMap := map[resourceKey]Resource{}
	deleteMap := map[resourceKey]Resource{}

	if len(opts.Only) > 0 && opts.Since != "" {
		_, _ = fmt.Fprintf(os.Stderr, "warning: --since is ignored when --only is set\n")
	}

	if scopeUsesAllFiles(opts, selection.hasCategory) {
		for _, resource := range allLocal {
			if !selection.Match(resource) {
				continue
			}
			uploadMap[keyFor(resource)] = resource
		}
	} else {
		gitChanges, err := CollectGitChanges(cfg, opts.Since)
		if err != nil {
			return nil, nil, err
		}
		if gitChanges.FallbackAll {
			for _, resource := range allLocal {
				if !selection.Match(resource) {
					continue
				}
				uploadMap[keyFor(resource)] = resource
			}
		} else {
			for _, changedPath := range gitChanges.Changed {
				resource, ok, err := ClassifyProjectPath(cfg, changedPath)
				if err != nil {
					return nil, nil, err
				}
				if !ok {
					continue
				}
				if current, exists := localByKey[keyFor(resource)]; exists {
					if !selection.Match(current) {
						continue
					}
					uploadMap[keyFor(current)] = current
				}
			}

			generated, err := CollectGenerated(cfg)
			if err != nil {
				return nil, nil, err
			}
			for _, resource := range generated {
				if current, exists := localByKey[keyFor(resource)]; exists {
					if !selection.Match(current) {
						continue
					}
					uploadMap[keyFor(current)] = current
				}
			}

			if mode == "sync" {
				for _, deletedPath := range gitChanges.Deleted {
					resource, ok, err := ClassifyProjectPath(cfg, deletedPath)
					if err != nil {
						return nil, nil, err
					}
					if ok && selection.Match(resource) {
						deleteMap[keyFor(resource)] = resource
					}
				}
			}
		}
	}

	if mode == "sync" && opts.Prune {
		remoteResources, err := FetchRemoteResources(ctx, client, cfg.Theme)
		if err != nil {
			return nil, nil, err
		}
		for _, remote := range remoteResources {
			if !remoteInManagedScope(cfg, remote) {
				continue
			}
			localPath, ok := localPathForRemote(cfg, remote)
			if ok {
				remote.LocalPath = localPath
			}
			if !selection.Match(remote) {
				continue
			}
			if _, exists := localByKey[keyFor(remote)]; exists {
				continue
			}
			deleteMap[keyFor(remote)] = remote
		}
	}

	return sortResourceSlice(uploadMap), sortResourceSlice(deleteMap), nil
}

// scopeUsesAllFiles returns true when the option/filter combination means we
// should iterate all local files instead of relying on git change detection.
func scopeUsesAllFiles(opts Options, hasCategory bool) bool {
	return opts.All || len(opts.Only) > 0 || (hasCategory && opts.Since == "")
}

func mapByKey(resources []Resource) map[resourceKey]Resource {
	mapped := make(map[resourceKey]Resource, len(resources))
	for _, resource := range resources {
		mapped[keyFor(resource)] = resource
	}
	return mapped
}

func remoteInManagedScope(cfg Config, resource Resource) bool {
	switch resource.Kind {
	case KindAsset:
		for _, root := range cfg.Roots {
			if root.Kind != KindAsset {
				continue
			}
			base := normalizePath(root.RemoteBase)
			if base == "" || base == "." {
				return true
			}
			if resource.RemoteName == base || hasPathPrefix(resource.RemoteName, base) {
				return true
			}
		}
		return false
	default:
		for _, root := range cfg.Roots {
			if root.Kind == resource.Kind {
				return true
			}
		}
		return false
	}
}

func hasPathPrefix(value, prefix string) bool {
	return value == prefix || len(value) > len(prefix) && value[:len(prefix)+1] == prefix+"/"
}

func sortResourceSlice(resources map[resourceKey]Resource) []Resource {
	items := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		items = append(items, resource)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].DisplayPath == items[j].DisplayPath {
			return items[i].Kind < items[j].Kind
		}
		return items[i].DisplayPath < items[j].DisplayPath
	})
	return items
}

func toActions(resources []Resource) []Action {
	actions := make([]Action, 0, len(resources))
	for _, resource := range resources {
		actions = append(actions, Action{
			DisplayPath: resource.DisplayPath,
			Kind:        resource.Kind,
			LocalPath:   resource.LocalPath,
			RemoteName:  resource.RemoteName,
		})
	}
	return actions
}
