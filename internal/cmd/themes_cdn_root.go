package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/output"
)

type themeCDNRootPayload struct {
	CDNRoot string `json:"cdn_root"`
	Site    string `json:"site"`
	Theme   string `json:"theme"`
}

// ThemesCDNRootCmd prints the resolved CDN root for the configured theme.
type ThemesCDNRootCmd struct {
	Theme string `help:"Override theme from nimbu.yml"`
}

// Run executes the cdn-root command.
func (c *ThemesCDNRootCmd) Run(ctx context.Context, flags *RootFlags) error {
	_, projectCfg, warnings, err := resolveThemeProjectConfig()
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: %s\n", warning)
	}

	theme := strings.TrimSpace(c.Theme)
	if theme == "" {
		theme = strings.TrimSpace(projectCfg.Theme)
	}
	if theme == "" {
		return fmt.Errorf("theme required; set theme in nimbu.yml or pass --theme")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	info, err := fetchThemeInfo(ctx, client, theme)
	if err != nil {
		return err
	}

	cdnRoot := strings.TrimSpace(info.CDNRoot)
	if cdnRoot == "" {
		return fmt.Errorf("theme %q info returned empty cdn_root", theme)
	}

	payload := themeCDNRootPayload{CDNRoot: cdnRoot, Site: site, Theme: theme}
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, payload)
	}

	return output.Plain(ctx, payload.CDNRoot)
}
