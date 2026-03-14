package migrate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/api"
)

// TranslationCopyOptions filters translations during copy.
type TranslationCopyOptions struct {
	Query  string
	Since  string
	DryRun bool
	Media  *MediaRewritePlan
}

// TranslationCopyItem describes one copied translation.
type TranslationCopyItem struct {
	Key    string `json:"key"`
	Action string `json:"action"`
}

// TranslationCopyResult reports translation copy work.
type TranslationCopyResult struct {
	From   SiteRef               `json:"from"`
	To     SiteRef               `json:"to"`
	Query  string                `json:"query"`
	Since  string                `json:"since,omitempty"`
	DryRun bool                  `json:"dry_run,omitempty"`
	Items  []TranslationCopyItem `json:"items,omitempty"`
}

// ParseSinceValue accepts RFC3339 or compact relative durations like 1d, 2w, 3h.
func ParseSinceValue(raw string, now time.Time) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts.UTC().Format(time.RFC3339), nil
	}
	unit := value[len(value)-1:]
	number := strings.TrimSpace(value[:len(value)-1])
	var multiplier time.Duration
	switch unit {
	case "s":
		multiplier = time.Second
	case "h":
		multiplier = time.Hour
	case "d":
		multiplier = 24 * time.Hour
	case "w":
		multiplier = 7 * 24 * time.Hour
	case "m":
		multiplier = 730 * time.Hour
	case "y":
		multiplier = 8766 * time.Hour
	default:
		return "", fmt.Errorf("invalid since value %q", raw)
	}
	var amount int
	if _, err := fmt.Sscanf(number, "%d", &amount); err != nil || amount <= 0 {
		return "", fmt.Errorf("invalid since value %q", raw)
	}
	return now.Add(-time.Duration(amount) * multiplier).UTC().Format(time.RFC3339), nil
}

// CopyTranslations copies translations between sites.
func CopyTranslations(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, opts TranslationCopyOptions) (TranslationCopyResult, error) {
	translations, err := listTranslations(ctx, fromClient, opts.Query, opts.Since)
	if err != nil {
		return TranslationCopyResult{From: fromRef, To: toRef, Query: opts.Query, Since: opts.Since, DryRun: opts.DryRun}, err
	}
	result := TranslationCopyResult{From: fromRef, To: toRef, Query: opts.Query, Since: opts.Since, DryRun: opts.DryRun}
	for i, translation := range translations {
		emitStageItem(ctx, "Translations", translation.Key, int64(i+1), int64(len(translations)))
		if opts.Media != nil {
			translation.Value = opts.Media.RewriteString("translations."+translation.Key+".value", translation.Value)
			for locale, value := range translation.Values {
				translation.Values[locale] = opts.Media.RewriteString("translations."+translation.Key+".values."+locale, value)
			}
		}
		action := "create"
		if opts.DryRun {
			action = "dry-run"
		} else if err := toClient.Post(ctx, "/translations", translation, &api.Translation{}); err != nil {
			return result, fmt.Errorf("copy translation %s: %w", translation.Key, err)
		}
		result.Items = append(result.Items, TranslationCopyItem{Key: translation.Key, Action: action})
	}
	return result, nil
}

func listTranslations(ctx context.Context, client *api.Client, query, since string) ([]api.Translation, error) {
	var opts []api.RequestOption
	switch {
	case strings.TrimSpace(query) == "", strings.TrimSpace(query) == "*":
	case strings.Contains(query, "*"):
		opts = append(opts, api.WithParam("key.start", strings.TrimSuffix(strings.TrimSpace(query), "*")))
	default:
		opts = append(opts, api.WithParam("key", strings.TrimSpace(query)))
	}
	if strings.TrimSpace(since) != "" {
		opts = append(opts, api.WithParam("updated_at.gte", since))
	}
	return api.List[api.Translation](ctx, client, "/translations", opts...)
}
