package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

// ThemePullCmd downloads managed remote theme files.
type ThemePullCmd struct {
	Theme      string `help:"Override theme from nimbu.yml"`
	LiquidOnly bool   `help:"Only download liquid resources" name:"liquid-only"`
}

// Run executes the pull command.
func (c *ThemePullCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "pull theme files"); err != nil {
		return err
	}

	projectRoot, projectCfg, warnings, err := resolveThemeProjectConfig()
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	resolved, err := themes.ResolveConfig(projectRoot, projectCfg, c.Theme)
	if err != nil {
		return err
	}
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	ctx, tl := syncWithTimeline(ctx, "pull", resolved.Theme, false)
	defer func() {
		if tl != nil {
			tl.Close()
		}
	}()

	result, err := themes.RunPull(ctx, client, resolved, themes.Options{LiquidOnly: c.LiquidOnly})
	if err != nil {
		return finishSyncTimelineError(tl, err)
	}

	if result.TimelineRendered {
		return nil
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, action := range result.Written {
			if _, err := output.Fprintln(ctx, action.LocalPath); err != nil {
				return err
			}
		}
		return nil
	}
	for _, action := range result.Written {
		if _, err := output.Fprintf(ctx, "write %s\n", action.LocalPath); err != nil {
			return err
		}
	}
	if _, err := output.Fprintf(ctx, "pull complete: %d files\n", len(result.Written)); err != nil {
		return err
	}
	return nil
}
