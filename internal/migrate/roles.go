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
func CopyRoles(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef) (RoleCopyResult, error) {
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

	for _, src := range srcRoles {
		payload := map[string]any{
			"name":        src.Name,
			"description": src.Description,
		}

		if existing, ok := targetByName[src.Name]; ok {
			var updated api.Role
			if err := toClient.Put(ctx, "/roles/"+url.PathEscape(existing.ID), payload, &updated); err != nil {
				return result, fmt.Errorf("update role %s: %w", src.Name, err)
			}
			result.Items = append(result.Items, RoleCopyItem{Name: src.Name, Action: "update"})
		} else {
			var created api.Role
			if err := toClient.Post(ctx, "/roles", payload, &created); err != nil {
				return result, fmt.Errorf("create role %s: %w", src.Name, err)
			}
			result.Items = append(result.Items, RoleCopyItem{Name: src.Name, Action: "create"})
		}
	}

	return result, nil
}
