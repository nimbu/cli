package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

// ThemeDiffCmd compares local liquid files with the remote theme.
type ThemeDiffCmd struct {
	Theme string `help:"Override theme from nimbu.yml"`
}

// Run executes the diff command.
func (c *ThemeDiffCmd) Run(ctx context.Context, flags *RootFlags) error {
	projectRoot, projectCfg, warnings, err := resolveThemeProjectConfig()
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
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

	result, err := themes.RunDiff(ctx, client, resolved)
	if err != nil {
		return err
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, item := range result.Entries {
			if _, err := output.Fprintf(ctx, "%s\t%s\n", item.Status, item.Path); err != nil {
				return err
			}
		}
		return nil
	}
	if len(result.Entries) == 0 {
		if _, err := output.Fprintln(ctx, "no differences found"); err != nil {
			return err
		}
		return nil
	}
	for _, item := range result.Entries {
		if _, err := output.Fprintf(ctx, "%s %s\n", item.Status, item.Path); err != nil {
			return err
		}
	}
	return nil
}
