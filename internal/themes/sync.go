package themes

import (
	"context"
	"fmt"
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
	result.Uploaded = toActions(uploads)
	result.Deleted = toActions(deletes)

	if opts.DryRun {
		return result, nil
	}

	task := output.ProgressFromContext(ctx).Counter(mode+" theme files", int64(len(uploads)+len(deletes)))
	for _, resource := range uploads {
		task.SetLabel("upload " + resource.DisplayPath)
		if err := Upsert(ctx, client, cfg.Theme, resource, opts.Force); err != nil {
			task.Fail(err)
			return result, fmt.Errorf("upload %s: %w", resource.DisplayPath, err)
		}
		task.Add(1)
	}
	for _, resource := range deletes {
		task.SetLabel("delete " + resource.DisplayPath)
		if err := Delete(ctx, client, cfg.Theme, resource); err != nil {
			task.Fail(err)
			return result, fmt.Errorf("delete %s: %w", resource.DisplayPath, err)
		}
		task.Add(1)
	}

	task.Done("done")
	return result, nil
}

func planOperations(ctx context.Context, client *api.Client, cfg Config, opts Options, mode string, allLocal []Resource, localByKey map[resourceKey]Resource, selection *selectionFilter) ([]Resource, []Resource, error) {
	uploadMap := map[resourceKey]Resource{}
	deleteMap := map[resourceKey]Resource{}

	if opts.All {
		for _, resource := range allLocal {
			if !selection.Match(resource) {
				continue
			}
			uploadMap[keyFor(resource)] = resource
		}
	} else {
		gitChanges, err := CollectGitChanges(cfg)
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
