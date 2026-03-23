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
}

type initPromptModel struct {
	Sites                []initSiteChoice
	Themes               []initThemeChoice
	BundleOptions        []bootstrap.Bundle
	RepeatableOptions    []bootstrap.Repeatable
	DefaultDirectoryName string
	OutputDir            string
	Source               string
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

	finalPath := filepath.Join(outputDir, answers.DirectoryName)
	result, err := bootstrap.BootstrapProject(bootstrap.BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: finalPath,
		Site:           selectedSite.ID,
		Theme:          selectedTheme.ID,
		BundleIDs:      answers.BundleIDs,
		RepeatableIDs:  answers.RepeatableIDs,
	})
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
		dirname := filepath.Base(result.Path)
		command := "cd " + dirname
		if install := detectInstallCommand(result.Path); install != "" {
			command += " && " + install
		}
		_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Out, "Done! To start working, run: %s\n", command)
		return nil
	}
}
