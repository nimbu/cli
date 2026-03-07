package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/output"
)

type appsPushResult struct {
	App     string            `json:"app"`
	Uploads []apps.FileAction `json:"uploads,omitempty"`
	Deletes []apps.FileAction `json:"deletes,omitempty"`
}

// AppsPushCmd pushes configured local cloud code files.
type AppsPushCmd struct {
	App   string   `help:"Configured local app name"`
	Sync  bool     `help:"Delete remote files missing locally"`
	Files []string `arg:"" optional:"" help:"Project-relative file subset"`
}

// Run executes the push command.
func (c *AppsPushCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "push app code"); err != nil {
		return err
	}
	if c.Sync && len(c.Files) > 0 {
		return fmt.Errorf("--sync cannot be combined with an explicit file subset")
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

	discovered, err := apps.CollectFiles(app)
	if err != nil {
		return fmt.Errorf("collect files: %w", err)
	}
	selected, err := apps.ExplicitFiles(discovered, c.Files)
	if err != nil {
		return err
	}
	ordered, err := apps.OrderFiles(app, selected)
	if err != nil {
		return err
	}
	result, deletes, err := apps.PlanPush(ctx, client, app, ordered, c.Sync)
	if err != nil {
		return fmt.Errorf("plan app push: %w", err)
	}
	if len(deletes) > 0 {
		ok, err := confirmPrompt(flags, fmt.Sprintf("delete %d remote app files", len(deletes)))
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("aborted")
		}
	}
	if err := apps.ExecutePush(ctx, client, app, result); err != nil {
		return err
	}

	outcome := appsPushResult{
		App:     app.Name,
		Uploads: append(append([]apps.FileAction{}, result.Updated...), result.Created...),
		Deletes: result.Deleted,
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, outcome)
	}
	if mode.Plain {
		for _, action := range outcome.Uploads {
			fmt.Printf("%s\t%s\n", action.Action, action.Name)
		}
		for _, action := range outcome.Deletes {
			fmt.Printf("%s\t%s\n", action.Action, action.Name)
		}
		return nil
	}
	for _, action := range outcome.Uploads {
		fmt.Printf("%s %s\n", action.Action, action.Name)
	}
	for _, action := range outcome.Deletes {
		fmt.Printf("%s %s\n", action.Action, action.Name)
	}
	fmt.Printf("push complete: %d uploads, %d deletes\n", len(outcome.Uploads), len(outcome.Deletes))
	return nil
}
