package themes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

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

	// Sort uploads in dependency order.
	orderedUploads, dependencyWarnings, err := orderUploadsByDependencies(uploads)
	if err != nil {
		return result, err
	}
	result.Warnings = append(result.Warnings, dependencyWarnings...)
	result.Deleted = toActions(deletes)
	categories := uploadCategoriesForOrderedResources(orderedUploads)

	tl := output.SyncTimelineFromContext(ctx)

	// Dry-run path
	if opts.DryRun {
		result.Uploaded = toActions(orderedUploads)
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
	// Categories are built from the same ordered upload stream, so the timeline
	// remains correct even if an unusual dependency creates repeated kind runs.
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
			if shouldHandleUploadConflict(err, opts) {
				overwrite, promptErr := promptOverwriteConflict(ctx, tl, opts, resource, err)
				if promptErr != nil {
					if tl != nil {
						tl.FileFailed(resource.DisplayPath, formatAPIError(promptErr))
						tl.ErrorFooter()
						result.TimelineRendered = true
					}
					return result, fmt.Errorf("confirm overwrite %s: %w", resource.DisplayPath, promptErr)
				}
				if !overwrite {
					result.Skipped = append(result.Skipped, toAction(resource))
					if tl != nil {
						tl.FileSkipped()
					}
					continue
				}
				if tl != nil {
					tl.SetActiveFile(resource.DisplayPath)
				}
				if err := Upsert(ctx, client, cfg.Theme, resource, true); err == nil {
					result.Uploaded = append(result.Uploaded, toAction(resource))
					if tl != nil {
						tl.FileUploaded()
					}
					continue
				} else {
					if tl != nil {
						tl.FileFailed(resource.DisplayPath, formatAPIError(err))
						tl.ErrorFooter()
						result.TimelineRendered = true
					}
					return result, fmt.Errorf("upload %s: %w", resource.DisplayPath, err)
				}
			}
			if tl != nil {
				tl.FileFailed(resource.DisplayPath, formatAPIError(err))
				tl.ErrorFooter()
				result.TimelineRendered = true
			}
			return result, fmt.Errorf("upload %s: %w", resource.DisplayPath, err)
		}
		result.Uploaded = append(result.Uploaded, toAction(resource))
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

func shouldHandleUploadConflict(err error, opts Options) bool {
	return !opts.Force && opts.ConfirmOverwrite != nil && isConflictError(err)
}

func promptOverwriteConflict(ctx context.Context, tl *output.SyncTimeline, opts Options, resource Resource, err error) (bool, error) {
	if tl != nil {
		tl.PreparePrompt()
	}
	return opts.ConfirmOverwrite(ctx, resource, err)
}

func isConflictError(err error) bool {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusConflict || strings.Contains(apiErr.Message, "Conflict")
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

func orderUploadsByDependencies(resources []Resource) ([]Resource, []string, error) {
	items := make([]ResourceContent, 0, len(resources))
	for _, resource := range resources {
		item := ResourceContent{Resource: resource}
		if resource.Kind != KindAsset {
			content, err := readFile(resource.AbsPath)
			if err != nil {
				return nil, nil, fmt.Errorf("read %s: %w", resource.DisplayPath, err)
			}
			item.Content = content
		}
		items = append(items, item)
	}
	orderedItems, warnings := OrderResourceContentByLiquidDependencies(items)
	ordered := make([]Resource, len(orderedItems))
	for i, item := range orderedItems {
		ordered[i] = item.Resource
	}
	return ordered, warnings, nil
}

func uploadCategoriesForOrderedResources(resources []Resource) []output.SyncCategory {
	if len(resources) == 0 {
		return nil
	}
	categories := make([]output.SyncCategory, 0, len(resources))
	for _, resource := range resources {
		label := resource.Kind.Collection()
		if len(categories) == 0 || categories[len(categories)-1].Label != label {
			categories = append(categories, output.SyncCategory{Label: label, Count: 1})
			continue
		}
		categories[len(categories)-1].Count++
	}
	return categories
}

func planOperations(ctx context.Context, client *api.Client, cfg Config, opts Options, mode string, allLocal []Resource, localByKey map[resourceKey]Resource, selection *selectionFilter) ([]Resource, []Resource, error) {
	uploadMap := map[resourceKey]Resource{}
	deleteMap := map[resourceKey]Resource{}

	if hasExplicitSelectors(opts) && opts.Since != "" {
		_, _ = fmt.Fprintf(os.Stderr, "warning: --since is ignored when explicit theme selectors are set\n")
	}

	var remoteResources []Resource
	if mode == "sync" && opts.Prune {
		var err error
		remoteResources, err = FetchRemoteResources(ctx, client, cfg.Theme)
		if err != nil {
			return nil, nil, err
		}
		for i := range remoteResources {
			if localPath, ok := localPathForRemote(cfg, remoteResources[i]); ok {
				remoteResources[i].LocalPath = localPath
			}
		}
	}

	if err := selection.validateExplicitSelectors(append(allLocal, remoteResources...)); err != nil {
		return nil, nil, err
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
		for _, remote := range remoteResources {
			if !remoteInManagedScope(cfg, remote) {
				continue
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
	return opts.All || hasExplicitSelectors(opts) || (hasCategory && opts.Since == "")
}

func hasExplicitSelectors(opts Options) bool {
	return len(opts.Only) > 0 || len(opts.Selectors) > 0
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
		actions = append(actions, toAction(resource))
	}
	return actions
}

func toAction(resource Resource) Action {
	return Action{
		DisplayPath: resource.DisplayPath,
		Kind:        resource.Kind,
		LocalPath:   resource.LocalPath,
		RemoteName:  resource.RemoteName,
	}
}
