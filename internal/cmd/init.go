package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/bootstrap"
	"github.com/nimbu/cli/internal/output"
)

type InitCmd struct {
	Directory string `arg:"" optional:"" help:"Target directory (use '.' for the current directory); created if it does not exist. Overrides --output-dir and skips the directory-name prompt."`
	Repo      string `help:"GitHub repo shorthand for the starterskit" default:"zenjoy/theme-starterskit"`
	Branch    string `help:"Branch to bootstrap from" default:"main"`
	Dir       string `help:"Use a local starterskit directory instead of cloning"`
	OutputDir string `help:"Parent directory for the new project"`
}

type initResult struct {
	Path        string   `json:"path"`
	Site        string   `json:"site"`
	Theme       string   `json:"theme"`
	Source      string   `json:"source"`
	Bundles     []string `json:"bundles,omitempty"`
	Repeatables []string `json:"repeatables,omitempty"`
}

type initSiteChoice struct {
	Label string
	Site  api.Site
}

type initThemeChoice struct {
	Label string
	Theme api.Theme
}

type initAnswers struct {
	SiteID         string
	ThemeID        string
	DirectoryName  string
	RepeatableMode string
	BundleIDs      []string
	RepeatableIDs  []string
	Confirmed      bool
}

type initPrompter interface {
	Run(initPromptModel) (initAnswers, error)
	// ResolveConflicts asks which of the conflicting (already-existing) files
	// may be overwritten. It returns the set of source-relative paths the user
	// declined to overwrite (kept as-is).
	ResolveConflicts(conflicts []string) (map[string]struct{}, error)
}

type initPromptModel struct {
	Sites                []initSiteChoice
	Themes               []initThemeChoice
	BundleOptions        []bootstrap.Bundle
	RepeatableOptions    []bootstrap.Repeatable
	DefaultDirectoryName string
	OutputDir            string
	Source               string
	// FixedTarget is the resolved absolute path when a positional directory
	// argument was given. When set, the directory-name prompt is skipped and
	// this path is used verbatim.
	FixedTarget string
}

func (c *InitCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "bootstrap project"); err != nil {
		return err
	}
	if flags != nil && flags.NoInput {
		return fmt.Errorf("init is interactive only; remove --no-input")
	}
	writer := output.WriterFromContext(ctx)
	if output.IsHuman(ctx) && writer != nil && writer.ErrIsTTY() && stdinIsTerminal() {
		return c.runInteractiveTTY(ctx, flags)
	}

	// Resolve and validate the positional target up front so an invalid target
	// fails before any starterskit clone.
	fixedTarget, fixed, err := c.resolveFixedTarget()
	if err != nil {
		return err
	}
	inPlace, err := targetIsExistingDir(fixed, fixedTarget)
	if err != nil {
		return err
	}

	newServerPresenter(ctx, false).PrintBanner()

	progress := output.ProgressFromContext(ctx)

	var cloneTask *output.Task
	if strings.TrimSpace(c.Dir) == "" {
		cloneTask = progress.Phase("Cloning starterskit")
	}
	sourceDir, sourceLabel, cleanup, err := c.resolveSource()
	if err != nil {
		if cloneTask != nil {
			cloneTask.Fail(err)
		}
		return err
	}
	if cloneTask != nil {
		cloneTask.Done("done")
	}
	defer cleanup()

	manifestTask := progress.Phase("Loading bootstrap manifest")
	manifest, err := loadInitManifest(sourceDir, sourceLabel)
	if err != nil {
		manifestTask.Fail(err)
		return err
	}
	manifestTask.Done("done")

	outputDir, err := c.resolveOutputDir()
	if err != nil {
		return err
	}

	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}
	sites, err := api.List[api.Site](ctx, client, "/sites")
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}
	if len(sites) == 0 {
		return fmt.Errorf("no accessible sites found")
	}

	prompter := newInitPrompter(ctx)
	promptModel := initPromptModel{
		Sites:                initSiteChoices(sites),
		BundleOptions:        manifest.Bundles,
		RepeatableOptions:    manifest.Repeatables,
		OutputDir:            outputDir,
		Source:               sourceLabel,
		DefaultDirectoryName: "theme-" + parameterize(sites[0].Subdomain),
		FixedTarget:          fixedTarget,
	}

	answers, err := prompter.Run(promptModel)
	if err != nil {
		return err
	}

	selectedSite, ok := findSiteByID(sites, answers.SiteID)
	if !ok {
		return fmt.Errorf("selected site %q not found", answers.SiteID)
	}

	themeClient, err := GetAPIClientWithSite(ctx, selectedSite.ID)
	if err != nil {
		return err
	}
	themes, err := api.List[api.Theme](ctx, themeClient, "/themes")
	if err != nil {
		return fmt.Errorf("list themes: %w", err)
	}
	if len(themes) == 0 {
		return fmt.Errorf("no themes found for site %s", selectedSite.ID)
	}

	promptModel.Sites = initSiteChoices([]api.Site{selectedSite})
	promptModel.Themes = initThemeChoices(themes)
	promptModel.DefaultDirectoryName = "theme-" + parameterize(selectedSite.Subdomain)
	answers, err = prompter.Run(promptModel)
	if err != nil {
		return err
	}
	if strings.TrimSpace(answers.DirectoryName) == "" {
		answers.DirectoryName = promptModel.DefaultDirectoryName
	}

	selectedTheme, ok := findThemeByID(themes, answers.ThemeID)
	if !ok {
		return fmt.Errorf("selected theme %q not found", answers.ThemeID)
	}

	finalPath := fixedTarget
	if !fixed {
		finalPath = filepath.Join(outputDir, answers.DirectoryName)
	}

	opts := bootstrap.BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: finalPath,
		Site:           siteConfigValue(selectedSite),
		Theme:          themeConfigValue(selectedTheme),
		BundleIDs:      answers.BundleIDs,
		RepeatableIDs:  answers.RepeatableIDs,
		AllowExisting:  inPlace,
	}
	if inPlace {
		conflicts, err := planConflicts(opts, finalPath)
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			skip, err := prompter.ResolveConflicts(conflicts)
			if err != nil {
				return err
			}
			opts.SkipPaths = skip
		}
	}

	result, err := bootstrap.BootstrapProject(opts)
	if err != nil {
		return err
	}

	return emitInitResult(ctx, initResult{
		Path:        result.Path,
		Site:        result.Site,
		Theme:       result.Theme,
		Source:      sourceLabel,
		Bundles:     result.Bundles,
		Repeatables: result.Repeatables,
	})
}

func (c *InitCmd) resolveSource() (string, string, func(), error) {
	if strings.TrimSpace(c.Dir) != "" {
		dir, err := filepath.Abs(c.Dir)
		if err != nil {
			return "", "", nil, fmt.Errorf("resolve --dir: %w", err)
		}
		if _, err := os.Stat(filepath.Join(dir, bootstrap.ManifestPath)); err != nil {
			return "", "", nil, fmt.Errorf("bootstrap manifest missing in %s", dir)
		}
		return dir, dir, func() {}, nil
	}

	tempDir, err := os.MkdirTemp("", "nimbu-init-*")
	if err != nil {
		return "", "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	sourceDir := filepath.Join(tempDir, "source")
	if err := cloneStarterRepo(sourceDir, c.Repo, c.Branch); err != nil {
		cleanup()
		return "", "", nil, err
	}
	return sourceDir, c.Repo + "@" + c.Branch, cleanup, nil
}

// resolveFixedTarget resolves the optional positional directory argument to an
// absolute path. The second return reports whether an explicit target was given
// — which skips the directory-name prompt and overrides --output-dir.
func (c *InitCmd) resolveFixedTarget() (string, bool, error) {
	raw := strings.TrimSpace(c.Directory)
	if raw == "" {
		return "", false, nil
	}
	dir, err := filepath.Abs(raw)
	if err != nil {
		return "", false, fmt.Errorf("resolve target directory %q: %w", raw, err)
	}
	return dir, true, nil
}

// targetIsExistingDir reports whether a fixed positional target already exists
// as a directory (→ in-place init). A target that exists but is not a directory
// is a user error and surfaces a clear message instead of a cryptic mkdir error.
func targetIsExistingDir(fixed bool, path string) (bool, error) {
	if !fixed {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // fresh target — will be created
		}
		return false, fmt.Errorf("inspect target %s: %w", path, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("target %s exists and is not a directory", path)
	}
	return true, nil
}

// planConflicts returns the source-relative paths that would be written and
// already exist under target.
func planConflicts(opts bootstrap.BootstrapOptions, target string) ([]string, error) {
	plan, err := bootstrap.PlanPaths(opts)
	if err != nil {
		return nil, err
	}
	var conflicts []string
	for _, rel := range plan {
		if fileExists(filepath.Join(target, filepath.FromSlash(rel))) {
			conflicts = append(conflicts, rel)
		}
	}
	return conflicts, nil
}

func (c *InitCmd) resolveOutputDir() (string, error) {
	raw := strings.TrimSpace(c.OutputDir)
	if raw == "" {
		raw = "."
	}
	dir, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve --output-dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", dir, err)
	}
	return dir, nil
}

func loadInitManifest(sourceDir, sourceLabel string) (bootstrap.Manifest, error) {
	manifestPath := filepath.Join(sourceDir, bootstrap.ManifestPath)
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			return bootstrap.Manifest{}, fmt.Errorf("bootstrap manifest missing for %s: expected %s; this repo/branch is not bootstrap-ready. Use --dir to point at a local starterskit checkout with bootstrap metadata", sourceLabel, manifestPath)
		}
		return bootstrap.Manifest{}, fmt.Errorf("stat bootstrap manifest: %w", err)
	}

	manifest, err := bootstrap.LoadManifest(sourceDir)
	if err != nil {
		return bootstrap.Manifest{}, err
	}
	return manifest, nil
}

func emitInitResult(ctx context.Context, result initResult) error {
	mode := output.FromContext(ctx)
	switch {
	case mode.JSON:
		return output.JSON(ctx, result)
	case mode.Plain:
		return output.Plain(ctx, result.Path, result.Site, result.Theme, result.Source)
	default:
		out := output.WriterFromContext(ctx).Out
		if command := initNextStepCommand(result.Path); command != "" {
			_, _ = fmt.Fprintf(out, "Done! To start working, run: %s\n", command)
		} else {
			_, _ = fmt.Fprintf(out, "Done! Your project is ready.\n")
		}
		return nil
	}
}

// initNextStepCommand returns the shell command to suggest after init, or "" when
// the project is the current directory and needs no install step (in-place init).
func initNextStepCommand(resultPath string) string {
	install := detectInstallCommand(resultPath)
	if initResultIsCWD(resultPath) {
		// Already inside the project directory — no `cd` needed.
		return install
	}
	command := "cd " + filepath.Base(resultPath)
	if install != "" {
		command += " && " + install
	}
	return command
}

func initResultIsCWD(path string) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	// Resolve symlinks so e.g. /tmp vs /private/tmp on macOS compare equal.
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return filepath.Clean(cwd) == filepath.Clean(abs)
}
