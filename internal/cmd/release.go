package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

// ReleaseCmd deploys the checked-out project and records successful releases.
type ReleaseCmd struct {
	Only       []string `help:"Release stages: schema,code,theme" name:"only"`
	DryRun     bool     `help:"Plan each selected stage without deploying" name:"dry-run"`
	AllowDirty bool     `help:"Allow uncommitted changes; mark the release dirty" name:"allow-dirty"`
	Yes        bool     `help:"Confirm protected environment deployment"`
}

type releaseRecord struct {
	ID          string         `json:"id,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
	Environment string         `json:"environment"`
	Commit      string         `json:"commit"`
	Ref         string         `json:"ref"`
	Actor       string         `json:"actor,omitempty"`
	Summary     map[string]any `json:"summary"`
	Forced      bool           `json:"forced"`
}

type releaseStage struct {
	name string
	run  func() (any, error)
}

func (c *ReleaseCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags == nil {
		flags = &RootFlags{}
	}
	names, err := releaseStageNames(c.Only)
	if err != nil {
		return err
	}
	if !c.DryRun {
		if err := requireWrite(flags, "release project"); err != nil {
			return err
		}
	}
	project, err := resolveProjectContext()
	if err != nil {
		return err
	}
	record, err := releaseGitRecord(ctx, project.ProjectRoot, c.AllowDirty)
	if err != nil {
		return err
	}
	record.Environment = flags.Environment
	record.Forced = c.Yes
	if !c.DryRun && names[0] != "schema" {
		if err := confirmProtectedEnvironment(ctx, flags, c.Yes, "release project"); err != nil {
			return err
		}
	}
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	if record.Environment == "" {
		record.Environment = site
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	if !c.DryRun {
		var existing []releaseRecord
		if err := client.Get(ctx, "/releases?per_page=1", &existing); err != nil {
			return fmt.Errorf("cannot verify release recording before deployment: %w", err)
		}
	}
	stageCtx := ctx
	if output.FromContext(ctx).JSON {
		writer := *output.WriterFromContext(ctx)
		writer.Out = io.Discard
		stageCtx = output.WithWriter(ctx, &writer)
	}
	stages := make([]releaseStage, 0, len(names))
	for _, name := range names {
		stages = append(stages, releaseStage{name: name, run: func() (any, error) {
			switch name {
			case "schema":
				if c.DryRun {
					command := &SchemaPlanCmd{}
					err := command.Run(stageCtx, flags)
					var exitErr *ExitError
					if errors.As(err, &exitErr) {
						if exitErr.Code == 2 || exitErr.Code == 3 {
							err = nil
						} else {
							err = newDetailedError(fmt.Errorf("schema plan is blocked; resolve the reported operations"), errorRequestInvalid, exitErr.Code, map[string]any{"plans": command.Plans})
						}
					}
					return releaseSchemaSummary(command.Plans, c.DryRun), err
				}
				command := &SchemaApplyCmd{Yes: c.Yes, DisableRenamePrompts: true}
				err := command.Run(stageCtx, flags)
				return releaseSchemaSummary(command.Plans, c.DryRun), err
			case "code":
				return releaseCode(stageCtx, client, project, flags, site, c.DryRun)
			default:
				return releaseTheme(stageCtx, client, project, flags, c.DryRun)
			}
		}})
	}
	return executeRelease(ctx, stages, &record, c.DryRun, func() error {
		return client.Post(ctx, "/releases", record, &record)
	})
}

func releaseStageNames(values []string) ([]string, error) {
	selected := splitRepeatedCSV(values)
	if len(selected) == 0 {
		return []string{"schema", "code", "theme"}, nil
	}
	wanted := map[string]bool{}
	for _, name := range selected {
		if name != "schema" && name != "code" && name != "theme" {
			return nil, fmt.Errorf("unknown release stage %q; use schema,code,theme", name)
		}
		wanted[name] = true
	}
	result := []string{}
	for _, name := range []string{"schema", "code", "theme"} {
		if wanted[name] {
			result = append(result, name)
		}
	}
	return result, nil
}

func releaseGitRecord(ctx context.Context, root string, allowDirty bool) (releaseRecord, error) {
	git := func(args ...string) (string, error) {
		command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
		out, err := command.Output()
		if err != nil {
			return "", fmt.Errorf("release requires a git checkout with a commit: %w", err)
		}
		return strings.TrimSpace(string(out)), nil
	}
	revision, err := git("rev-parse", "--verify", "HEAD")
	if err != nil {
		return releaseRecord{}, err
	}
	status, err := git("status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return releaseRecord{}, err
	}
	dirty := status != ""
	if dirty && !allowDirty {
		return releaseRecord{}, fmt.Errorf("working tree is dirty; commit changes or use --allow-dirty")
	}
	ref, err := git("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return releaseRecord{}, err
	}
	return releaseRecord{Commit: revision, Ref: ref, Summary: map[string]any{"dirty": dirty}}, nil
}

func executeRelease(ctx context.Context, stages []releaseStage, record *releaseRecord, dryRun bool, save func() error) error {
	completed := []string{}
	for i, stage := range stages {
		summary, err := stage.run()
		if err != nil {
			pending := []string{}
			for _, later := range stages[i+1:] {
				pending = append(pending, later.name)
			}
			return fmt.Errorf("release failed at %s: %w; completed: %s; unattempted: %s; earlier changes remain applied; inspect and re-plan before retrying", stage.name, err, strings.Join(completed, ","), strings.Join(pending, ","))
		}
		record.Summary[stage.name] = summary
		completed = append(completed, stage.name)
	}
	if dryRun {
		if output.FromContext(ctx).JSON {
			return output.JSON(ctx, map[string]any{"dry_run": true, "release": record})
		}
		_, err := output.Fprintf(ctx, "release dry-run complete: %s\n", strings.Join(completed, ", "))
		return err
	}
	if err := save(); err != nil {
		return fmt.Errorf("deployment completed (%s), but release recording failed: %w; do not redeploy merely to retry recording", strings.Join(completed, ", "), err)
	}
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, record)
	}
	_, err := output.Fprintf(ctx, "release recorded: %s (%s)\n", record.Commit, strings.Join(completed, ", "))
	return err
}

func releaseCode(ctx context.Context, client *api.Client, project projectContext, flags *RootFlags, site string, dryRun bool) (any, error) {
	results := []apps.Result{}
	configured := apps.VisibleApps(project.ProjectRoot, project.Config, currentAPIHost(flags), site)
	if len(project.Config.Apps) > 0 && len(configured) == 0 {
		return nil, fmt.Errorf("no configured apps match release host/site; configure target apps or omit code with --only")
	}
	for _, app := range configured {
		files, err := apps.CollectFiles(app)
		if err != nil {
			return nil, err
		}
		ordered, err := apps.OrderFiles(app, files)
		if err != nil {
			return nil, err
		}
		result, _, err := apps.PlanPush(ctx, client, app, ordered, false)
		if err != nil {
			return nil, err
		}
		if !dryRun {
			if err := apps.ExecutePush(ctx, client, app, result); err != nil {
				return nil, err
			}
		}
		results = append(results, result)
	}
	if dryRun {
		if err := output.JSON(ctx, map[string]any{"code": results}); err != nil {
			return nil, err
		}
	}
	return map[string]any{"apps": results}, nil
}

func releaseTheme(ctx context.Context, client *api.Client, project projectContext, flags *RootFlags, dryRun bool) (any, error) {
	resolved, err := themes.ResolveConfig(project.ProjectRoot, project.Config, "")
	if err != nil {
		return nil, err
	}
	result, err := themes.RunPush(ctx, client, resolved, themes.Options{All: true, DryRun: dryRun, Force: flags.Force, ConfirmOverwrite: confirmThemeOverwrite(flags)})
	if err != nil {
		return nil, err
	}
	if len(result.Skipped) > 0 {
		return nil, fmt.Errorf("theme deployment incomplete: %d files skipped", len(result.Skipped))
	}
	if err := writeThemeTransferResult(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
}

func releaseSchemaSummary(plans []schemaPlan, dryRun bool) map[string]any {
	counts := map[string]int{}
	changed := 0
	for _, plan := range plans {
		if len(plan.Ops) > 0 {
			changed++
		}
		for _, op := range plan.Ops {
			counts[fmt.Sprint(op["kind"])]++
		}
	}
	result := map[string]any{"targets": len(plans), "changed_targets": changed, "operations": counts}
	if dryRun {
		result["plans"] = plans
	}
	return result
}
