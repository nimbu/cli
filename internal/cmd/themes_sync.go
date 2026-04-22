package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

var themeConflictRE = regexp.MustCompile(`Conflict \((.*)\)`)

// ThemePushCmd uploads managed local theme files without deleting remote files.
type ThemePushCmd struct {
	All        bool     `help:"Upload all managed theme files"`
	Build      bool     `help:"Run sync.build before collecting files"`
	DryRun     bool     `help:"Print planned uploads without changing remote state" name:"dry-run"`
	Since      string   `help:"Compare against this git ref instead of HEAD (e.g. origin/main)"`
	Theme      string   `help:"Override theme from nimbu.yml"`
	Only       []string `help:"Only upload these project-relative files" name:"only"`
	LiquidOnly bool     `help:"Only upload liquid resources" name:"liquid-only"`
	CSSOnly    bool     `help:"Only upload stylesheet assets" name:"css-only"`
	JSOnly     bool     `help:"Only upload JavaScript assets" name:"js-only"`
	ImagesOnly bool     `help:"Only upload image assets" name:"images-only"`
	FontsOnly  bool     `help:"Only upload font assets" name:"fonts-only"`
}

// Run executes the push command.
func (c *ThemePushCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runThemeTransfer(ctx, flags, c.Theme, themes.Options{
		All:        c.All,
		Build:      c.Build,
		DryRun:     c.DryRun,
		Force:      flags != nil && flags.Force,
		Since:      c.Since,
		Only:       c.Only,
		LiquidOnly: c.LiquidOnly,
		CSSOnly:    c.CSSOnly,
		JSOnly:     c.JSOnly,
		ImagesOnly: c.ImagesOnly,
		FontsOnly:  c.FontsOnly,
	}, "push")
}

// ThemeSyncCmd uploads managed local theme files and optionally deletes remote files.
type ThemeSyncCmd struct {
	All        bool     `help:"Upload all managed theme files"`
	Build      bool     `help:"Run sync.build before collecting files"`
	DryRun     bool     `help:"Print planned uploads/deletes without changing remote state" name:"dry-run"`
	Prune      bool     `help:"Delete managed remote theme files missing locally"`
	Since      string   `help:"Compare against this git ref instead of HEAD (e.g. origin/main)"`
	Theme      string   `help:"Override theme from nimbu.yml"`
	Only       []string `help:"Only sync these project-relative files" name:"only"`
	LiquidOnly bool     `help:"Only sync liquid resources" name:"liquid-only"`
	CSSOnly    bool     `help:"Only sync stylesheet assets" name:"css-only"`
	JSOnly     bool     `help:"Only sync JavaScript assets" name:"js-only"`
	ImagesOnly bool     `help:"Only sync image assets" name:"images-only"`
	FontsOnly  bool     `help:"Only sync font assets" name:"fonts-only"`
}

// Run executes the sync command.
func (c *ThemeSyncCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runThemeTransfer(ctx, flags, c.Theme, themes.Options{
		All:        c.All,
		Build:      c.Build,
		DryRun:     c.DryRun,
		Force:      flags != nil && flags.Force,
		Prune:      c.Prune,
		Since:      c.Since,
		Only:       c.Only,
		LiquidOnly: c.LiquidOnly,
		CSSOnly:    c.CSSOnly,
		JSOnly:     c.JSOnly,
		ImagesOnly: c.ImagesOnly,
		FontsOnly:  c.FontsOnly,
	}, "sync")
}

func runThemeTransfer(ctx context.Context, flags *RootFlags, themeOverride string, opts themes.Options, mode string) error {
	if (!opts.DryRun || opts.Build) && flags != nil {
		if err := requireWrite(flags, mode+" theme files"); err != nil {
			return err
		}
	}

	projectRoot, projectCfg, warnings, err := resolveThemeProjectConfig()
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	resolved, err := themes.ResolveConfig(projectRoot, projectCfg, themeOverride)
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

	ctx, tl := syncWithTimeline(ctx, mode, resolved.Theme, opts.DryRun)
	defer func() {
		if tl != nil {
			tl.Close()
		}
	}()

	var result themes.Result
	opts.ConfirmOverwrite = confirmThemeOverwrite(flags)
	if mode == "sync" {
		result, err = themes.RunSync(ctx, client, resolved, opts)
	} else {
		result, err = themes.RunPush(ctx, client, resolved, opts)
	}
	if err != nil {
		return finishSyncTimelineError(tl, err)
	}
	return writeThemeTransferResult(ctx, result)
}

func confirmThemeOverwrite(flags *RootFlags) func(context.Context, themes.Resource, error) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	return func(ctx context.Context, resource themes.Resource, err error) (bool, error) {
		if flags != nil && flags.NoInput {
			return false, fmt.Errorf("use --force to overwrite %s", resource.DisplayPath)
		}
		w := output.WriterFromContext(ctx)
		if detail := themeConflictDetail(err); detail != "" {
			_, _ = fmt.Fprintf(w.Err, "warning: %s\n", detail)
		}
		_, _ = fmt.Fprintf(w.Err, "overwrite %s? [y/N]: ", resource.DisplayPath)
		answer, readErr := reader.ReadString('\n')
		if readErr != nil && len(answer) == 0 {
			return false, readErr
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			_, _ = fmt.Fprintf(w.Err, "forcing upload of %s\n", resource.DisplayPath)
			return true, nil
		default:
			_, _ = fmt.Fprintf(w.Err, "skipping upload of %s\n", resource.DisplayPath)
			return false, nil
		}
	}
}

func themeConflictDetail(err error) string {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return ""
	}
	if match := themeConflictRE.FindStringSubmatch(apiErr.Message); len(match) == 2 {
		return match[1]
	}
	if apiErr.StatusCode == http.StatusConflict {
		return apiErr.Message
	}
	return ""
}

func resolveThemeProjectConfig() (string, config.ProjectConfig, []string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", config.ProjectConfig{}, nil, err
	}

	projectRoot := cwd
	var projectCfg config.ProjectConfig
	var warnings []string

	projectFile, err := config.FindProjectFile()
	if err == nil {
		projectRoot = filepath.Dir(projectFile)
		projectCfg, err = config.ReadProjectConfigFrom(projectFile)
		if err != nil {
			return "", config.ProjectConfig{}, nil, fmt.Errorf("read project config: %w", err)
		}
		if keyWarnings, warnErr := config.WarnUnknownSyncKeys(projectFile); warnErr == nil {
			warnings = append(warnings, keyWarnings...)
		}
	} else if !errors.Is(err, config.ErrNotFound) {
		return "", config.ProjectConfig{}, nil, err
	}

	return projectRoot, projectCfg, warnings, nil
}

func writeThemeTransferResult(ctx context.Context, result themes.Result) error {
	if result.TimelineRendered {
		return nil
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, action := range result.Uploaded {
			if _, err := output.Fprintf(ctx, "upload\t%s\n", action.DisplayPath); err != nil {
				return err
			}
		}
		for _, action := range result.Deleted {
			if _, err := output.Fprintf(ctx, "delete\t%s\n", action.DisplayPath); err != nil {
				return err
			}
		}
		for _, action := range result.Skipped {
			if _, err := output.Fprintf(ctx, "skip\t%s\n", action.DisplayPath); err != nil {
				return err
			}
		}
		return nil
	}

	prefix := ""
	if result.DryRun {
		prefix = "[dry-run] "
	}
	for _, action := range result.Uploaded {
		if _, err := output.Fprintf(ctx, "%supload %s\n", prefix, action.DisplayPath); err != nil {
			return err
		}
	}
	for _, action := range result.Deleted {
		if _, err := output.Fprintf(ctx, "%sdelete %s\n", prefix, action.DisplayPath); err != nil {
			return err
		}
	}
	for _, action := range result.Skipped {
		if _, err := output.Fprintf(ctx, "%sskip %s\n", prefix, action.DisplayPath); err != nil {
			return err
		}
	}
	skipped := ""
	if len(result.Skipped) > 0 {
		skipped = fmt.Sprintf(", %d skipped", len(result.Skipped))
	}
	if _, err := output.Fprintf(ctx, "%s complete: %d uploads, %d deletes%s\n", result.Mode, len(result.Uploaded), len(result.Deleted), skipped); err != nil {
		return err
	}
	return nil
}
