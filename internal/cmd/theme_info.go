package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

type themeInfoPayload struct {
	CDNBasePath  string `json:"cdn_base_path"`
	CDNHost      string `json:"cdn_host"`
	CDNRoot      string `json:"cdn_root"`
	SiteID       string `json:"site_id"`
	SiteShortID  string `json:"site_short_id"`
	ThemeID      string `json:"theme_id"`
	ThemeShortID string `json:"theme_short_id"`
}

func fetchThemeInfo(ctx context.Context, client *api.Client, theme string) (themeInfoPayload, error) {
	var info themeInfoPayload
	path := "/themes/" + url.PathEscape(theme) + "/info"
	if err := client.Get(ctx, path, &info); err != nil {
		return themeInfoPayload{}, fmt.Errorf("get theme info: %w", err)
	}
	return info, nil
}

func applyThemeInfo(theme *api.Theme, info themeInfoPayload) {
	if theme == nil {
		return
	}

	if value := strings.TrimSpace(info.CDNBasePath); value != "" {
		theme.CDNBasePath = value
	}
	if value := strings.TrimSpace(info.CDNHost); value != "" {
		theme.CDNHost = value
	}
	if value := strings.TrimSpace(info.CDNRoot); value != "" {
		theme.CDNRoot = value
	}
	if value := strings.TrimSpace(info.SiteID); value != "" {
		theme.SiteID = value
	}
	if value := strings.TrimSpace(info.SiteShortID); value != "" {
		theme.SiteShortID = value
	}
	if value := strings.TrimSpace(info.ThemeShortID); value != "" {
		theme.ThemeShortID = value
	}
	if value := strings.TrimSpace(info.ThemeID); value != "" && strings.TrimSpace(theme.ID) == "" {
		theme.ID = value
	}
}
