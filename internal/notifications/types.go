package notifications

import "github.com/nimbu/cli/internal/api"

const RelativeRoot = "content/notifications"

// Template models one notification template on disk.
type Template struct {
	Slug         string
	Name         string
	Description  string
	Subject      string
	Text         string
	HTML         string
	HTMLEnabled  bool
	Translations map[string]Translation
}

// Translation models one localized override on disk.
type Translation struct {
	Subject string
	Text    string
	HTML    string
}

// FallbackLocales mirrors the legacy locale allowlist.
var FallbackLocales = []string{
	"ar", "bg", "ca", "cs", "da", "de", "el", "en", "es", "et", "eu",
	"fa", "fi", "fr", "ga", "gl", "hi", "hr", "hu", "is", "it", "ja",
	"ko", "lb", "lt", "lv", "nl", "no", "pl", "pt", "ro", "ru", "sl",
	"sr", "sv", "th", "tr", "zh",
}

// PullResult reports files written during pull.
type PullResult struct {
	Root    string   `json:"root"`
	Slugs   []string `json:"slugs"`
	Written []string `json:"written"`
}

// PushAction reports one uploaded notification.
type PushAction struct {
	Action string `json:"action"`
	Slug   string `json:"slug"`
}

// PushResult reports notifications uploaded during push.
type PushResult struct {
	Root    string       `json:"root"`
	Actions []PushAction `json:"actions"`
}

// ToNotification converts a disk template into the API contract.
func (t Template) ToNotification() api.Notification {
	notification := api.Notification{
		Slug:        t.Slug,
		Name:        t.Name,
		Description: t.Description,
		Subject:     t.Subject,
		Text:        t.Text,
		HTML:        t.HTML,
		HTMLEnabled: t.HTMLEnabled,
	}
	if len(t.Translations) > 0 {
		notification.Translations = make(map[string]map[string]any, len(t.Translations))
		for locale, translation := range t.Translations {
			item := map[string]any{
				"text": translation.Text,
			}
			if translation.Subject != "" {
				item["subject"] = translation.Subject
			}
			if translation.HTML != "" {
				item["html"] = translation.HTML
			}
			notification.Translations[locale] = item
		}
	}
	return notification
}
