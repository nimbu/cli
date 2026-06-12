package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/output"
)

// AppsCodePullCmd pulls remote app code files into the configured local app directory.
type AppsCodePullCmd struct {
	App   string   `help:"Configured local app name"`
	Files []string `help:"Remote or project-relative file subset; commas split multiple files" name:"only"`
}

// Run executes apps code pull.
func (c *AppsCodePullCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "pull app code"); err != nil {
		return err
	}

	project, err := resolveProjectContext()
	if err != nil {
		return err
	}
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	activeHost := currentAPIHost(flags)
	configuredApps := apps.VisibleApps(project.ProjectRoot, project.Config, activeHost, site)
	app, err := apps.ResolveApp(configuredApps, c.App)
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	result, err := apps.Pull(ctx, client, app, apps.PullOptions{Only: splitRepeatedCSV(c.Files)})
	if err != nil {
		return fmt.Errorf("pull app code: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, action := range result.Written {
			if _, err := output.Fprintln(ctx, action.Local); err != nil {
				return err
			}
		}
		return nil
	}
	for _, action := range result.Written {
		if _, err := output.Fprintf(ctx, "write %s\n", action.Local); err != nil {
			return err
		}
	}
	_, err = output.Fprintf(ctx, "pull complete: %d files\n", len(result.Written))
	return err
}
