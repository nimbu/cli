package migrate

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
)

// RoleCopyItem describes one copied role.
type RoleCopyItem struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

// RoleCopyResult reports role copy results.
type RoleCopyResult struct {
	From  SiteRef        `json:"from"`
	To    SiteRef        `json:"to"`
	Items []RoleCopyItem `json:"items,omitempty"`
}

// CopyRoles copies customer roles between sites.
func CopyRoles(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, dryRun bool) (RoleCopyResult, error) {
	result := RoleCopyResult{From: fromRef, To: toRef}

	srcRoles, err := api.List[api.Role](ctx, fromClient, "/roles")
	if err != nil {
		return result, fmt.Errorf("list source roles: %w", err)
	}

	dstRoles, err := api.List[api.Role](ctx, toClient, "/roles")
	if err != nil {
		return result, fmt.Errorf("list target roles: %w", err)
	}

	targetByName := make(map[string]api.Role, len(dstRoles))
	for _, r := range dstRoles {
		targetByName[r.Name] = r
	}

	for i, src := range srcRoles {
		emitStageItem(ctx, "Roles", src.Name, int64(i+1), int64(len(srcRoles)))
		if _, ok := targetByName[src.Name]; ok {
			action := "update"
			if dryRun {
				action = "dry-run:" + action
			} else {
				payload := map[string]any{
					"name":        src.Name,
					"description": src.Description,
				}
				var updated api.Role
				if err := toClient.Put(ctx, "/roles/"+url.PathEscape(targetByName[src.Name].ID), payload, &updated); err != nil {
					return result, fmt.Errorf("update role %s: %w", src.Name, err)
				}
			}
			result.Items = append(result.Items, RoleCopyItem{Name: src.Name, Action: action})
		} else {
			action := "create"
			if dryRun {
				action = "dry-run:" + action
			} else {
				payload := map[string]any{
					"name":        src.Name,
					"description": src.Description,
				}
				var created api.Role
				if err := toClient.Post(ctx, "/roles", payload, &created); err != nil {
					return result, fmt.Errorf("create role %s: %w", src.Name, err)
				}
			}
			result.Items = append(result.Items, RoleCopyItem{Name: src.Name, Action: action})
		}
	}

	return result, nil
}
