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

	result, err := themes.RunPull(ctx, client, resolved, themes.Options{LiquidOnly: c.LiquidOnly})
	if err != nil {
		return err
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, action := range result.Written {
			fmt.Println(action.LocalPath)
		}
		return nil
	}
	for _, action := range result.Written {
		fmt.Printf("write %s\n", action.LocalPath)
	}
	fmt.Printf("pull complete: %d files\n", len(result.Written))
	return nil
}
