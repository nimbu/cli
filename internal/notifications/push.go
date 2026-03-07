package notifications

import (
	"context"
	"fmt"
	"net/url"
	"sort"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BuildNotifications reads disk templates and converts them to API notifications.
func BuildNotifications(root string, only map[string]struct{}, allowedLocales map[string]struct{}) ([]api.Notification, error) {
	templates, err := ReadTemplates(root, only, allowedLocales)
	if err != nil {
		return nil, err
	}
	items := make([]api.Notification, 0, len(templates))
	for _, template := range templates {
		items = append(items, template.ToNotification())
	}
	return items, nil
}

// Push uploads local notifications using safe upsert semantics.
func Push(ctx context.Context, client *api.Client, root string, allowedLocales map[string]struct{}, only map[string]struct{}) (PushResult, error) {
	items, err := BuildNotifications(root, only, allowedLocales)
	if err != nil {
		return PushResult{}, err
	}

	result := PushResult{Root: root}
	task := output.ProgressFromContext(ctx).Counter("push notifications", int64(len(items)))
	for _, notification := range items {
		task.SetLabel("push " + notification.Slug)
		path := "/notifications/" + url.PathEscape(notification.Slug)

		var stored api.Notification
		action := "create"
		if err := client.Get(ctx, path, &stored); err == nil {
			if err := client.Put(ctx, path, ToNotificationPayload(fromNotification(notification)), &stored); err != nil {
				task.Fail(err)
				return PushResult{}, fmt.Errorf("update %s: %w", notification.Slug, err)
			}
			action = "update"
		} else if api.IsNotFound(err) {
			if err := client.Post(ctx, "/notifications", ToNotificationPayload(fromNotification(notification)), &stored); err != nil {
				task.Fail(err)
				return PushResult{}, fmt.Errorf("create %s: %w", notification.Slug, err)
			}
		} else {
			task.Fail(err)
			return PushResult{}, fmt.Errorf("load %s: %w", notification.Slug, err)
		}

		result.Actions = append(result.Actions, PushAction{Action: action, Slug: notification.Slug})
		task.Add(1)
	}
	sort.SliceStable(result.Actions, func(i, j int) bool {
		return result.Actions[i].Slug < result.Actions[j].Slug
	})
	task.Done("done")
	return result, nil
}

// ToNotificationPayload converts one local template into API payload form.
func ToNotificationPayload(tpl Template) map[string]any {
	body := map[string]any{
		"description": tpl.Description,
		"name":        tpl.Name,
		"slug":        tpl.Slug,
		"subject":     tpl.Subject,
		"text":        tpl.Text,
	}
	if tpl.HTMLEnabled {
		body["html_enabled"] = true
		body["html"] = tpl.HTML
	}
	if len(tpl.Translations) > 0 {
		translations := map[string]any{}
		for locale, item := range tpl.Translations {
			entry := map[string]any{"text": item.Text}
			if item.Subject != "" {
				entry["subject"] = item.Subject
			}
			if item.HTML != "" {
				entry["html"] = item.HTML
			}
			translations[locale] = entry
		}
		body["translations"] = translations
	}
	return body
}

// ToNotification converts a local template into api.Notification for output.
func ToNotification(tpl Template) api.Notification {
	item := api.Notification{
		Description: tpl.Description,
		HTML:        tpl.HTML,
		HTMLEnabled: tpl.HTMLEnabled,
		Name:        tpl.Name,
		Slug:        tpl.Slug,
		Subject:     tpl.Subject,
		Text:        tpl.Text,
	}
	if len(tpl.Translations) > 0 {
		item.Translations = map[string]map[string]any{}
		for locale, tr := range tpl.Translations {
			item.Translations[locale] = map[string]any{
				"html":    tr.HTML,
				"subject": tr.Subject,
				"text":    tr.Text,
			}
		}
	}
	return item
}

func fromNotification(notification api.Notification) Template {
	template := Template{
		Description:  notification.Description,
		HTML:         notification.HTML,
		HTMLEnabled:  notification.HTMLEnabled,
		Name:         notification.Name,
		Slug:         notification.Slug,
		Subject:      notification.Subject,
		Text:         notification.Text,
		Translations: map[string]Translation{},
	}
	for locale, raw := range notification.Translations {
		template.Translations[locale] = Translation{
			HTML:    stringValue(raw["html"]),
			Subject: stringValue(raw["subject"]),
			Text:    stringValue(raw["text"]),
		}
	}
	return template
}
