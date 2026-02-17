package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// TranslationsListCmd lists translations.
type TranslationsListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *TranslationsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	requestFlags := *flags
	requestFlags.Locale = ""

	opts, err := listRequestOptions(&requestFlags)
	if err != nil {
		return fmt.Errorf("list translations: %w", err)
	}

	var translations []api.Translation
	if c.All {
		translations, err = api.List[api.Translation](ctx, client, "/translations", opts...)
	} else {
		var paged *api.PagedResponse[api.Translation]
		paged, err = api.ListPage[api.Translation](ctx, client, "/translations", c.Page, c.PerPage, opts...)
		if err == nil {
			translations = paged.Data
		}
	}
	if err != nil {
		return fmt.Errorf("list translations: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, translations)
	}

	translations = expandTranslationsListRows(translations, flags.Locale)

	plainFields := []string{"key", "locale", "value"}
	tableFields := []string{"key", "locale", "value"}
	tableHeaders := []string{"KEY", "LOCALE", "VALUE"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, translations, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, translations, fields, headers)
}

func expandTranslationsListRows(translations []api.Translation, localeFilter string) []api.Translation {
	rows := make([]api.Translation, 0, len(translations))

	for _, translation := range translations {
		if translation.Locale != "" || translation.Value != "" || len(translation.Values) == 0 {
			rows = append(rows, translation)
			continue
		}

		if localeFilter != "" {
			value, ok := translationValueForLocale(translation.Values, localeFilter)
			if !ok {
				continue
			}
			rows = append(rows, api.Translation{Key: translation.Key, Locale: localeFilter, Value: value})
			continue
		}

		locales := make([]string, 0, len(translation.Values))
		for locale := range translation.Values {
			locales = append(locales, locale)
		}
		sort.Strings(locales)

		for _, locale := range locales {
			rows = append(rows, api.Translation{Key: translation.Key, Locale: locale, Value: translation.Values[locale]})
		}
	}

	return rows
}

func translationValueForLocale(values map[string]string, localeFilter string) (string, bool) {
	if value, ok := values[localeFilter]; ok {
		return value, true
	}

	normalizedFilter := normalizeLocale(localeFilter)
	if normalizedFilter == "" {
		return "", false
	}

	for locale, value := range values {
		if normalizeLocale(locale) == normalizedFilter {
			return value, true
		}
	}

	for locale, value := range values {
		normalizedLocale := normalizeLocale(locale)
		if strings.SplitN(normalizedLocale, "-", 2)[0] == normalizedFilter {
			return value, true
		}
	}

	return "", false
}
