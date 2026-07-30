package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/nimbu/cli/internal/api"
)

// ConsentCopyOptions controls consent configuration copying.
type ConsentCopyOptions struct {
	DryRun                     bool
	PlannedTargetPageFullpaths map[string]struct{}
}

// ConsentCopyResult reports one whole consent configuration copy.
type ConsentCopyResult struct {
	From     SiteRef        `json:"from"`
	To       SiteRef        `json:"to"`
	Action   string         `json:"action"`
	Config   map[string]any `json:"config,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

// CopyConsentConfig copies the complete consent configuration without
// rewriting its field-level locale maps.
func CopyConsentConfig(
	ctx context.Context,
	fromClient, toClient *api.Client,
	fromRef, toRef SiteRef,
	opts ConsentCopyOptions,
) (ConsentCopyResult, error) {
	result := ConsentCopyResult{From: fromRef, To: toRef}
	var raw json.RawMessage
	if err := fromClient.Get(ctx, "/settings/consent", &raw); err != nil {
		return result, fmt.Errorf("read source consent configuration: %w", err)
	}
	config := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&config); err != nil {
		return result, fmt.Errorf("decode source consent configuration: %w", err)
	}
	if err := remapConsentPrivacyPage(ctx, fromClient, toClient, config, opts); err != nil {
		return result, err
	}

	result.Action = "update"
	result.Config = config
	if opts.DryRun {
		result.Action = "dry-run:update"
		return result, nil
	}
	var response map[string]any
	if err := toClient.Put(ctx, "/settings/consent", config, &response); err != nil {
		return result, fmt.Errorf("replace target consent configuration: %w", err)
	}
	return result, nil
}

func remapConsentPrivacyPage(ctx context.Context, fromClient, toClient *api.Client, config map[string]any, opts ConsentCopyOptions) error {
	kind, _ := config["privacy_policy_kind"].(string)
	if kind != "page" {
		return nil
	}
	sourceID, ok := config["privacy_policy_page_id"].(string)
	if !ok || sourceID == "" {
		return fmt.Errorf("page-based consent configuration requires a nonempty string privacy_policy_page_id")
	}

	sourcePages, err := listPageSummaries(ctx, fromClient, "*")
	if err != nil {
		return fmt.Errorf("list source pages for consent privacy policy: %w", err)
	}
	fullpath := ""
	for _, page := range sourcePages {
		if page.ID == sourceID {
			fullpath = page.Fullpath
			break
		}
	}
	if fullpath == "" {
		return fmt.Errorf("source privacy policy page ID %q was not found; cannot safely copy consent configuration", sourceID)
	}

	targetPages, err := listPageSummaries(ctx, toClient, "*")
	if err != nil {
		return fmt.Errorf("list target pages for consent privacy policy %q: %w", fullpath, err)
	}
	for _, page := range targetPages {
		if page.Fullpath == fullpath && page.ID != "" {
			config["privacy_policy_page_id"] = page.ID
			return nil
		}
	}
	if opts.DryRun {
		if _, planned := opts.PlannedTargetPageFullpaths[api.NormalizePageFullpath(fullpath)]; planned {
			return nil
		}
	}
	return fmt.Errorf("target privacy policy page %q was not found after page copy; cannot safely copy consent configuration", fullpath)
}
