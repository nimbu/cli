package notifications

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// Pull fetches remote notifications and writes them to disk.
func Pull(ctx context.Context, client *api.Client, root string, only map[string]struct{}) (PullResult, error) {
	notifications, err := api.List[api.Notification](ctx, client, "/notifications")
	if err != nil {
		return PullResult{}, err
	}
	selected := 0
	for _, notification := range notifications {
		if len(only) > 0 {
			if _, ok := only[notification.Slug]; !ok {
				continue
			}
		}
		selected++
	}
	task := output.ProgressFromContext(ctx).Counter("pull notifications", int64(selected))
	written, err := WriteNotifications(root, notifications, only)
	if err != nil {
		task.Fail(err)
		return PullResult{}, err
	}
	var slugs []string
	for _, notification := range notifications {
		if len(only) > 0 {
			if _, ok := only[notification.Slug]; !ok {
				continue
			}
		}
		slugs = append(slugs, notification.Slug)
		task.SetLabel("write " + notification.Slug)
		task.Add(1)
	}
	sort.Strings(slugs)
	task.Done("done")
	return PullResult{
		Root:    root,
		Slugs:   slugs,
		Written: written,
	}, nil
}

// WriteNotifications writes remote notifications to the local disk contract.
func WriteNotifications(root string, notifications []api.Notification, only map[string]struct{}) ([]string, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}

	sort.SliceStable(notifications, func(i, j int) bool {
		return notifications[i].Slug < notifications[j].Slug
	})

	var written []string
	for _, notification := range notifications {
		if len(only) > 0 {
			if _, ok := only[notification.Slug]; !ok {
				continue
			}
		}

		baseContent, err := encodeFrontMatter([]frontMatterField{
			{Key: "description", Value: notification.Description},
			{Key: "name", Value: notification.Name},
			{Key: "subject", Value: notification.Subject},
		}, notification.Text)
		if err != nil {
			return nil, err
		}
		basePath := filepath.Join(root, notification.Slug+".txt")
		if err := os.WriteFile(basePath, baseContent, 0o644); err != nil {
			return nil, err
		}
		written = append(written, basePath)

		if notification.HTMLEnabled && notification.HTML != "" {
			htmlPath := filepath.Join(root, notification.Slug+".html")
			if err := os.WriteFile(htmlPath, []byte(notification.HTML), 0o644); err != nil {
				return nil, err
			}
			written = append(written, htmlPath)
		}

		wrote, err := writeTranslations(root, notification)
		if err != nil {
			return nil, err
		}
		written = append(written, wrote...)
	}

	sort.Strings(written)
	return written, nil
}

func writeTranslations(root string, notification api.Notification) ([]string, error) {
	if len(notification.Translations) == 0 {
		return nil, nil
	}

	locales := make([]string, 0, len(notification.Translations))
	for locale := range notification.Translations {
		locales = append(locales, locale)
	}
	sort.Strings(locales)

	var written []string
	for _, locale := range locales {
		translation := notification.Translations[locale]
		text := stringValue(translation["text"])
		html := stringValue(translation["html"])
		subject := stringValue(translation["subject"])
		if text == notification.Text && html == notification.HTML && subject == notification.Subject {
			continue
		}

		localeRoot := filepath.Join(root, locale)
		if err := os.MkdirAll(localeRoot, 0o755); err != nil {
			return nil, err
		}

		txt, err := encodeFrontMatter([]frontMatterField{{Key: "subject", Value: subject}}, text)
		if err != nil {
			return nil, err
		}
		txtPath := filepath.Join(localeRoot, notification.Slug+".txt")
		if err := os.WriteFile(txtPath, txt, 0o644); err != nil {
			return nil, err
		}
		written = append(written, txtPath)

		if notification.HTMLEnabled && html != "" {
			htmlPath := filepath.Join(localeRoot, notification.Slug+".html")
			if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
				return nil, err
			}
			written = append(written, htmlPath)
		}
	}
	return written, nil
}
