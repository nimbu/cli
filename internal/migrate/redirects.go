package migrate

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
)

// RedirectCopyItem describes one copied redirect.
type RedirectCopyItem struct {
	Source string `json:"source"`
	Action string `json:"action"`
}

// RedirectCopyResult reports redirect copy results.
type RedirectCopyResult struct {
	From  SiteRef            `json:"from"`
	To    SiteRef            `json:"to"`
	Items []RedirectCopyItem `json:"items,omitempty"`
}

// CopyRedirects copies URL redirects between sites.
func CopyRedirects(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef) (RedirectCopyResult, error) {
	result := RedirectCopyResult{From: fromRef, To: toRef}

	sourceRedirects, err := api.List[api.Redirect](ctx, fromClient, "/redirects")
	if err != nil {
		return result, fmt.Errorf("list source redirects: %w", err)
	}

	targetRedirects, err := api.List[api.Redirect](ctx, toClient, "/redirects")
	if err != nil {
		return result, fmt.Errorf("list target redirects: %w", err)
	}

	targetBySource := make(map[string]api.Redirect, len(targetRedirects))
	for _, r := range targetRedirects {
		targetBySource[r.Source] = r
	}

	for _, r := range sourceRedirects {
		payload := map[string]any{
			"source": r.Source,
			"target": r.Target,
		}
		if existing, ok := targetBySource[r.Source]; ok {
			path := "/redirects/" + url.PathEscape(existing.ID)
			if err := toClient.Put(ctx, path, payload, nil); err != nil {
				return result, fmt.Errorf("update redirect %s: %w", r.Source, err)
			}
			result.Items = append(result.Items, RedirectCopyItem{Source: r.Source, Action: "update"})
		} else {
			if err := toClient.Post(ctx, "/redirects", payload, nil); err != nil {
				return result, fmt.Errorf("create redirect %s: %w", r.Source, err)
			}
			result.Items = append(result.Items, RedirectCopyItem{Source: r.Source, Action: "create"})
		}
	}

	return result, nil
}
