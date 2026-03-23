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

	// Single-locale or no values: use Detail for all modes.
	if t.Locale != "" || t.Value != "" || len(t.Values) == 0 {
		return output.Detail(ctx, t, []any{t.Key, t.Locale, t.Value}, []output.Field{
			output.FAlways("Key", t.Key),
			output.FAlways("Locale", t.Locale),
			output.FAlways("Value", t.Value),
		})
	}

	// Multi-locale: JSON and Plain have dedicated paths; human mode renders locale list.
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, t)
	}

	if mode.Plain {
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

	if _, err := output.Fprintf(ctx, "Key:    %s\n", t.Key); err != nil {
		return err
	}
	if _, err := output.Fprintln(ctx, "Values:"); err != nil {
		return err
	}
	locales := make([]string, 0, len(t.Values))
	for locale := range t.Values {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	for _, locale := range locales {
		if _, err := output.Fprintf(ctx, "  %s: %s\n", locale, t.Values[locale]); err != nil {
			return err
		}
	}

	return nil
}
