package cmd

import (
	"context"
	"fmt"
	"net/url"
	"sort"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// TranslationsGetCmd gets a translation.
type TranslationsGetCmd struct {
	Key string `arg:"" help:"Translation key"`
}

// Run executes the get command.
func (c *TranslationsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var t api.Translation
	path := "/translations/" + url.PathEscape(c.Key)
	if err := client.Get(ctx, path, &t); err != nil {
		return fmt.Errorf("get translation: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, t)
	}

	if mode.Plain {
		if t.Locale == "" && t.Value == "" && len(t.Values) > 0 {
			rows := make([][]any, 0, len(t.Values))
			locales := make([]string, 0, len(t.Values))
			for locale := range t.Values {
				locales = append(locales, locale)
			}
			sort.Strings(locales)
			for _, locale := range locales {
				rows = append(rows, []any{t.Key, locale, t.Values[locale]})
			}
			return output.PlainRows(ctx, rows)
		}
		return output.Plain(ctx, t.Key, t.Locale, t.Value)
	}

	fmt.Printf("Key:    %s\n", t.Key)
	if t.Locale != "" || t.Value != "" || len(t.Values) == 0 {
		fmt.Printf("Locale: %s\n", t.Locale)
		fmt.Printf("Value:  %s\n", t.Value)
		return nil
	}

	fmt.Println("Values:")
	locales := make([]string, 0, len(t.Values))
	for locale := range t.Values {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	for _, locale := range locales {
		fmt.Printf("  %s: %s\n", locale, t.Values[locale])
	}

	return nil
}
