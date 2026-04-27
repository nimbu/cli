package themes

import (
	"context"
	"fmt"
	"sort"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/observer"
)

// CopyOptions controls one copy run.
type CopyOptions struct {
	DryRun          bool
	Force           bool
	LiquidOnly      bool
	ContinueOnError bool
}

// CopyRef identifies one site/theme/host tuple.
type CopyRef struct {
	BaseURL string `json:"base_url"`
	Site    string `json:"site"`
	Theme   string `json:"theme"`
}

// CopyResult records copied resources.
type CopyResult struct {
	From     CopyRef  `json:"from"`
	To       CopyRef  `json:"to"`
	Items    []Action `json:"items,omitempty"`
	Skipped  []Action `json:"skipped,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// RunCopy copies one theme between two clients.
func RunCopy(ctx context.Context, fromClient *api.Client, from CopyRef, toClient *api.Client, to CopyRef, opts CopyOptions) (CopyResult, error) {
	remoteResources, err := FetchRemoteResources(ctx, fromClient, from.Theme)
	if err != nil {
		return CopyResult{From: from, To: to}, err
	}
	remoteResources = SortByKindOrder(remoteResources)

	result := CopyResult{From: from, To: to}
	obs := observer.ObserverFromContext(ctx)
	var uploadItems []ResourceContent
	for _, resource := range remoteResources {
		if opts.LiquidOnly && resource.Kind == KindAsset {
			continue
		}
		item := ResourceContent{Resource: resource}
		if resource.Kind != KindAsset && !opts.DryRun {
			content, err := readResourceContent(ctx, fromClient, from.Theme, resource)
			if err != nil {
				if opts.ContinueOnError {
					warning := fmt.Sprintf("skip %s: read: %v", resource.DisplayPath, err)
					result.Warnings = append(result.Warnings, warning)
					result.Skipped = append(result.Skipped, toAction(resource))
					obs.StageWarning("Theme", warning)
					continue
				}
				return result, fmt.Errorf("read %s: %w", resource.DisplayPath, err)
			}
			item.Content = content
		}
		uploadItems = append(uploadItems, item)
	}

	orderedItems, graphWarnings := OrderResourceContentByLiquidDependencies(uploadItems)
	result.Warnings = append(result.Warnings, graphWarnings...)
	for _, warning := range graphWarnings {
		obs.StageWarning("Theme", warning)
	}

	total := int64(len(orderedItems))
	for i, item := range orderedItems {
		resource := item.Resource
		obs.StageItem("Theme", resource.DisplayPath, int64(i+1), total)
		if !opts.DryRun {
			content := item.Content
			if resource.Kind == KindAsset {
				var err error
				content, err = readResourceContent(ctx, fromClient, from.Theme, resource)
				if err != nil {
					if opts.ContinueOnError {
						warning := fmt.Sprintf("skip %s: read: %v", resource.DisplayPath, err)
						result.Warnings = append(result.Warnings, warning)
						result.Skipped = append(result.Skipped, toAction(resource))
						obs.StageWarning("Theme", warning)
						continue
					}
					return result, fmt.Errorf("read %s: %w", resource.DisplayPath, err)
				}
			}
			if err := UpsertBytes(ctx, toClient, to.Theme, resource, content, opts.Force); err != nil {
				if opts.ContinueOnError {
					warning := fmt.Sprintf("skip %s: upload: %v", resource.DisplayPath, err)
					result.Warnings = append(result.Warnings, warning)
					result.Skipped = append(result.Skipped, toAction(resource))
					obs.StageWarning("Theme", warning)
					continue
				}
				return result, fmt.Errorf("upload %s: %w", resource.DisplayPath, err)
			}
		}
		result.Items = append(result.Items, toAction(resource))
	}
	sort.SliceStable(result.Items, func(i, j int) bool { return result.Items[i].DisplayPath < result.Items[j].DisplayPath })
	sort.SliceStable(result.Skipped, func(i, j int) bool { return result.Skipped[i].DisplayPath < result.Skipped[j].DisplayPath })
	return result, nil
}
