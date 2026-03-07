package themes

import (
	"context"
	"fmt"
	"sort"

	"github.com/nimbu/cli/internal/api"
)

// CopyOptions controls one copy run.
type CopyOptions struct {
	Force      bool
	LiquidOnly bool
}

// CopyRef identifies one site/theme/host tuple.
type CopyRef struct {
	BaseURL string `json:"base_url"`
	Site    string `json:"site"`
	Theme   string `json:"theme"`
}

// CopyResult records copied resources.
type CopyResult struct {
	From  CopyRef  `json:"from"`
	To    CopyRef  `json:"to"`
	Items []Action `json:"items,omitempty"`
}

// RunCopy copies one theme between two clients.
func RunCopy(ctx context.Context, fromClient *api.Client, from CopyRef, toClient *api.Client, to CopyRef, opts CopyOptions) (CopyResult, error) {
	remoteResources, err := FetchRemoteResources(ctx, fromClient, from.Theme)
	if err != nil {
		return CopyResult{From: from, To: to}, err
	}
	sort.SliceStable(remoteResources, func(i, j int) bool {
		return remoteResources[i].DisplayPath < remoteResources[j].DisplayPath
	})

	result := CopyResult{From: from, To: to}
	for _, resource := range remoteResources {
		if opts.LiquidOnly && resource.Kind == KindAsset {
			continue
		}
		content, err := readResourceContent(ctx, fromClient, from.Theme, resource)
		if err != nil {
			return result, fmt.Errorf("read %s: %w", resource.DisplayPath, err)
		}
		if err := UpsertBytes(ctx, toClient, to.Theme, resource, content, opts.Force); err != nil {
			return result, fmt.Errorf("upload %s: %w", resource.DisplayPath, err)
		}
		result.Items = append(result.Items, Action{
			DisplayPath: resource.DisplayPath,
			Kind:        resource.Kind,
			RemoteName:  resource.RemoteName,
		})
	}
	sort.SliceStable(result.Items, func(i, j int) bool { return result.Items[i].DisplayPath < result.Items[j].DisplayPath })
	return result, nil
}
