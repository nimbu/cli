package notifications

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/config"
)

// ResolveProjectRoot returns the nearest nimbu.yml directory or cwd.
func ResolveProjectRoot() (string, error) {
	projectFile, err := config.FindProjectFile()
	if err == nil {
		return filepath.Dir(projectFile), nil
	}
	if err != nil && err != config.ErrNotFound {
		return "", err
	}
	return os.Getwd()
}

// RootPath returns the notifications directory for a project root.
func RootPath(projectRoot string) string {
	return filepath.Join(projectRoot, filepath.FromSlash(RelativeRoot))
}

// AllowedLocales builds the accepted locale set.
func AllowedLocales(siteLocales []string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(siteLocales)+len(FallbackLocales))
	for _, locale := range siteLocales {
		locale = strings.TrimSpace(locale)
		if locale != "" {
			allowed[locale] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		for _, locale := range FallbackLocales {
			allowed[locale] = struct{}{}
		}
	}
	return allowed
}

// ReadTemplates reads local notifications from disk.
func ReadTemplates(root string, only map[string]struct{}, allowedLocales map[string]struct{}) ([]Template, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var templates []Template
	var locales []string
	for _, entry := range entries {
		if entry.IsDir() {
			locale := entry.Name()
			if strings.HasPrefix(locale, ".") {
				continue
			}
			if _, ok := allowedLocales[locale]; ok {
				locales = append(locales, locale)
				continue
			}
			return nil, fmt.Errorf("%s: unsupported locale directory %q", root, locale)
		}
		if filepath.Ext(entry.Name()) != ".txt" {
			continue
		}

		slug := strings.TrimSuffix(entry.Name(), ".txt")
		if len(only) > 0 {
			if _, ok := only[slug]; !ok {
				continue
			}
		}

		template, err := readBaseTemplate(root, slug)
		if err != nil {
			return nil, err
		}
		if len(locales) > 0 {
			template.Translations, err = readTranslations(root, slug, locales)
			if err != nil {
				return nil, err
			}
		}
		templates = append(templates, template)
	}

	sort.SliceStable(templates, func(i, j int) bool {
		return templates[i].Slug < templates[j].Slug
	})
	return templates, nil
}

func readBaseTemplate(root, slug string) (Template, error) {
	path := filepath.Join(root, slug+".txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return Template{}, err
	}
	attrs, body, err := parseFrontMatter(data)
	if err != nil {
		return Template{}, err
	}

	name := strings.TrimSpace(stringValue(attrs["name"]))
	description := strings.TrimSpace(stringValue(attrs["description"]))
	subject := strings.TrimSpace(stringValue(attrs["subject"]))
	switch {
	case name == "":
		return Template{}, fmt.Errorf("%s: name is missing", path)
	case description == "":
		return Template{}, fmt.Errorf("%s: description is missing", path)
	case subject == "":
		return Template{}, fmt.Errorf("%s: subject is missing", path)
	}

	template := Template{
		Slug:        slug,
		Name:        name,
		Description: description,
		Subject:     subject,
		Text:        body,
	}

	htmlPath := filepath.Join(root, slug+".html")
	if data, err := os.ReadFile(htmlPath); err == nil {
		template.HTMLEnabled = true
		template.HTML = string(data)
	} else if err != nil && !os.IsNotExist(err) {
		return Template{}, err
	}

	return template, nil
}

func readTranslations(root, slug string, locales []string) (map[string]Translation, error) {
	translations := map[string]Translation{}
	sort.Strings(locales)
	for _, locale := range locales {
		path := filepath.Join(root, locale, slug+".txt")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		attrs, body, err := parseFrontMatter(data)
		if err != nil {
			return nil, err
		}
		translation := Translation{
			Subject: strings.TrimSpace(stringValue(attrs["subject"])),
			Text:    body,
		}
		htmlPath := filepath.Join(root, locale, slug+".html")
		if html, err := os.ReadFile(htmlPath); err == nil {
			translation.HTML = string(html)
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		translations[locale] = translation
	}
	if len(translations) == 0 {
		return nil, nil
	}
	return translations, nil
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
