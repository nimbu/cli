package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

// ThemeCopyCmd copies a theme between sites/themes.
type ThemeCopyCmd struct {
	From       string `help:"Source site[/theme]" required:"" name:"from"`
	To         string `help:"Target site[/theme]" required:"" name:"to"`
	FromHost   string `help:"Source API base URL or host" name:"from-host"`
	ToHost     string `help:"Target API base URL or host" name:"to-host"`
	LiquidOnly bool   `help:"Only copy liquid resources" name:"liquid-only"`
}

// Run executes themes copy.
func (c *ThemeCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "copy theme"); err != nil {
		return err
	}

	fromRef, err := parseThemeCopyRef(ctx, c.From, c.FromHost)
	if err != nil {
		return err
	}
	toRef, err := parseThemeCopyRef(ctx, c.To, c.ToHost)
	if err != nil {
		return err
	}

	fromClient, err := newAPIClientForBase(ctx, fromRef.BaseURL, fromRef.Site)
	if err != nil {
		return err
	}
	toClient, err := newAPIClientForBase(ctx, toRef.BaseURL, toRef.Site)
	if err != nil {
		return err
	}

	ctx, tl := copyWithTimeline(ctx, "Theme", fromRef.Site, toRef.Site, false)
	if tl != nil {
		defer tl.Close()
	}
	result, err := themes.RunCopy(ctx, fromClient, fromRef, toClient, toRef, themes.CopyOptions{
		Force:      flags != nil && flags.Force,
		LiquidOnly: c.LiquidOnly,
	})
	if err != nil {
		return finishCopyTimelineError(tl, err)
	}
	finishCopyTimeline(tl, "Theme", fmt.Sprintf("%d assets", len(result.Items)))

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, item := range result.Items {
			fmt.Println(item.DisplayPath)
		}
		return nil
	}
	if tl != nil {
		return nil
	}
	for _, item := range result.Items {
		fmt.Printf("copy %s\n", item.DisplayPath)
	}
	fmt.Printf("copy complete: %d files\n", len(result.Items))
	return nil
}

func parseThemeCopyRef(ctx context.Context, raw, hostOverride string) (themes.CopyRef, error) {
	site, theme := splitThemeTarget(raw)
	if site == "" {
		return themes.CopyRef{}, fmt.Errorf("invalid site/theme: %s", raw)
	}

	baseURL := strings.TrimSpace(hostOverride)
	if baseURL == "" {
		flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
		baseURL = flags.APIURL
	}
	if !strings.Contains(baseURL, "://") {
		baseURL = "https://" + strings.TrimPrefix(baseURL, "api.")
		if !strings.Contains(baseURL, "api.") {
			baseURL = strings.Replace(baseURL, "https://", "https://api.", 1)
		}
	}
	return themes.CopyRef{BaseURL: baseURL, Site: site, Theme: theme}, nil
}

func splitThemeTarget(raw string) (string, string) {
	value := strings.Trim(strings.TrimSpace(raw), "/")
	if value == "" {
		return "", ""
	}
	parts := strings.SplitN(value, "/", 2)
	if len(parts) == 1 {
		return parts[0], "default-theme"
	}
	if parts[1] == "" {
		return parts[0], "default-theme"
	}
	return parts[0], parts[1]
}
