package migrate

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

// NotificationCopyItem describes one copied notification.
type NotificationCopyItem struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Action string `json:"action"`
}

// NotificationCopyResult reports notification copy results.
type NotificationCopyResult struct {
	From  SiteRef                `json:"from"`
	To    SiteRef                `json:"to"`
	Query string                 `json:"query"`
	Items []NotificationCopyItem `json:"items,omitempty"`
}

// CopyNotifications copies notification templates between sites.
func CopyNotifications(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, query string, media *MediaRewritePlan) (NotificationCopyResult, error) {
	notifications, err := listNotifications(ctx, fromClient, query)
	if err != nil {
		return NotificationCopyResult{From: fromRef, To: toRef, Query: query}, err
	}
	result := NotificationCopyResult{From: fromRef, To: toRef, Query: query}
	for _, n := range notifications {
		slug := n.Slug
		if slug == "" {
			continue
		}
		payload := map[string]any{
			"slug":         n.Slug,
			"name":         n.Name,
			"description":  n.Description,
			"subject":      n.Subject,
			"text":         n.Text,
			"html":         n.HTML,
			"html_enabled": n.HTMLEnabled,
			"translations": n.Translations,
		}
		if media != nil {
			prefix := "notifications." + slug
			payload["subject"] = media.RewriteString(prefix+".subject", n.Subject)
			payload["text"] = media.RewriteString(prefix+".text", n.Text)
			payload["html"] = media.RewriteString(prefix+".html", n.HTML)
			payload["translations"] = media.RewriteValue(prefix+".translations", n.Translations)
		}

		var existing api.Notification
		path := "/notifications/" + url.PathEscape(slug)
		err := toClient.Get(ctx, path, &existing)
		switch {
		case err == nil:
			if err := toClient.Put(ctx, path, payload, &existing); err != nil {
				return result, fmt.Errorf("update notification %s: %w", slug, err)
			}
			result.Items = append(result.Items, NotificationCopyItem{Slug: slug, Name: n.Name, Action: "update"})
		case api.IsNotFound(err):
			if err := toClient.Post(ctx, "/notifications", payload, &existing); err != nil {
				return result, fmt.Errorf("create notification %s: %w", slug, err)
			}
			result.Items = append(result.Items, NotificationCopyItem{Slug: slug, Name: n.Name, Action: "create"})
		default:
			return result, err
		}
	}
	return result, nil
}

func listNotifications(ctx context.Context, client *api.Client, query string) ([]api.Notification, error) {
	query = strings.TrimSpace(query)
	var opts []api.RequestOption
	if query != "" && query != "*" {
		opts = append(opts, api.WithParam("slug", query))
	}
	return api.List[api.Notification](ctx, client, "/notifications", opts...)
}
