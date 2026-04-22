package migrate

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

type siteLocaleInfo struct {
	DefaultLocale string
	Locales       []string
}

func sharedNonDefaultContentLocales(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef) ([]string, []string) {
	source, sourceErr := fetchSiteLocaleInfo(ctx, fromClient, fromRef.Site)
	target, targetErr := fetchSiteLocaleInfo(ctx, toClient, toRef.Site)
	var warnings []string
	if sourceErr != nil {
		warnings = append(warnings, fmt.Sprintf("source site locales fetch failed: %v; localized channel fields will be skipped", sourceErr))
	}
	if targetErr != nil {
		warnings = append(warnings, fmt.Sprintf("target site locales fetch failed: %v; localized channel fields will be skipped", targetErr))
	}
	if sourceErr != nil || targetErr != nil {
		return nil, warnings
	}
	return sharedNonDefaultLocales(source.Locales, target.Locales, source.DefaultLocale, target.DefaultLocale), warnings
}

func fetchSiteLocaleInfo(ctx context.Context, client *api.Client, site string) (siteLocaleInfo, error) {
	if client == nil {
		return siteLocaleInfo{}, fmt.Errorf("missing API client")
	}
	site = strings.TrimSpace(site)
	if site == "" {
		site = strings.TrimSpace(client.Site)
	}
	if site == "" {
		return siteLocaleInfo{}, fmt.Errorf("site required")
	}
	var raw map[string]any
	if err := client.Get(ctx, "/sites/"+url.PathEscape(site)+"/settings", &raw); err != nil {
		return siteLocaleInfo{}, err
	}

	info := siteLocaleInfo{
		DefaultLocale: stringValue(raw["default_locale"]),
		Locales:       stringSliceValue(raw["locales"]),
	}
	if len(info.Locales) == 0 {
		info.Locales = stringSliceValue(raw["available_locales"])
	}
	if len(info.Locales) == 0 && info.DefaultLocale != "" {
		info.Locales = []string{info.DefaultLocale}
	}
	if info.DefaultLocale == "" && len(info.Locales) > 0 {
		info.DefaultLocale = info.Locales[0]
	}
	return info, nil
}

func sharedNonDefaultLocales(source, target []string, defaultLocales ...string) []string {
	defaultSet := make(map[string]struct{}, len(defaultLocales))
	for _, locale := range defaultLocales {
		locale = strings.TrimSpace(locale)
		if locale != "" {
			defaultSet[locale] = struct{}{}
		}
	}
	targetSet := make(map[string]struct{}, len(target))
	for _, locale := range target {
		locale = strings.TrimSpace(locale)
		if locale != "" {
			targetSet[locale] = struct{}{}
		}
	}
	var shared []string
	seen := map[string]struct{}{}
	for _, locale := range source {
		locale = strings.TrimSpace(locale)
		if locale == "" {
			continue
		}
		if _, ok := defaultSet[locale]; ok {
			continue
		}
		if _, ok := targetSet[locale]; !ok {
			continue
		}
		if _, ok := seen[locale]; ok {
			continue
		}
		seen[locale] = struct{}{}
		shared = append(shared, locale)
	}
	sort.Strings(shared)
	return shared
}

func stringSliceValue(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text := stringValue(value); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func channelsHaveLocalizedFields(channelMap map[string]api.ChannelDetail, channels []string) bool {
	for _, channel := range channels {
		detail, ok := channelMap[channel]
		if !ok {
			continue
		}
		for _, field := range detail.Customizations {
			if field.Localized {
				return true
			}
		}
	}
	return false
}
