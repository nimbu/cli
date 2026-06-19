package cmd

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/bootstrap"
)

func (m *initTeaModel) filteredOptions() []initChoiceOption {
	query := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))
	options := m.allOptions()
	if query == "" {
		return options
	}

	filtered := make([]initChoiceOption, 0, len(options))
	for _, option := range options {
		if strings.Contains(strings.ToLower(option.Label), query) || strings.Contains(strings.ToLower(option.ID), query) {
			filtered = append(filtered, option)
		}
	}
	return filtered
}

func (m *initTeaModel) allOptions() []initChoiceOption {
	switch m.step {
	case initTeaStepSite:
		return siteOptions(m.prompt.Sites)
	case initTeaStepTheme:
		return themeOptions(m.prompt.Themes)
	case initTeaStepRepeatableMode:
		return []initChoiceOption{
			{ID: "none", Label: "None"},
			{ID: "all", Label: "All repeatables"},
			{ID: "select", Label: "Select repeatables"},
		}
	case initTeaStepRepeatables:
		return repeatableOptions(m.prompt.RepeatableOptions)
	case initTeaStepBundles:
		return bundleOptions(m.prompt.BundleOptions)
	case initTeaStepOverwrite:
		return conflictOptions(m.conflicts)
	default:
		return nil
	}
}

func conflictOptions(conflicts []string) []initChoiceOption {
	out := make([]initChoiceOption, 0, len(conflicts))
	for _, rel := range conflicts {
		out = append(out, initChoiceOption{ID: rel, Label: rel})
	}
	return out
}

func (m *initTeaModel) isSelected(id string) bool {
	switch m.step {
	case initTeaStepRepeatables:
		_, ok := m.repeatables[id]
		return ok
	case initTeaStepBundles:
		_, ok := m.bundles[id]
		return ok
	case initTeaStepOverwrite:
		_, ok := m.overwrite[id]
		return ok
	default:
		return false
	}
}

func (m *initTeaModel) resolveSourceCmd() tea.Cmd {
	return func() tea.Msg {
		sourceDir, sourceLabel, cleanup, err := m.cmd.resolveSource()
		if err != nil {
			return initTeaErrMsg{err: err}
		}
		outputDir, err := m.cmd.resolveOutputDir()
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return initTeaErrMsg{err: err}
		}
		return initTeaSourceResolvedMsg{
			sourceDir:   sourceDir,
			sourceLabel: sourceLabel,
			outputDir:   outputDir,
			cleanup:     cleanup,
		}
	}
}

func (m *initTeaModel) loadManifestCmd() tea.Cmd {
	sourceDir := m.sourceDir
	sourceLabel := m.sourceLabel
	return func() tea.Msg {
		manifest, err := loadInitManifest(sourceDir, sourceLabel)
		if err != nil {
			return initTeaErrMsg{err: err}
		}
		return initTeaManifestLoadedMsg{manifest: manifest}
	}
}

func (m *initTeaModel) loadSitesCmd() tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		client, err := GetAPIClient(ctx)
		if err != nil {
			return initTeaErrMsg{err: err}
		}
		sites, err := api.List[api.Site](ctx, client, "/sites")
		if err != nil {
			return initTeaErrMsg{err: fmt.Errorf("list sites: %w", err)}
		}
		return initTeaSitesLoadedMsg{sites: sites}
	}
}

func (m *initTeaModel) loadThemesCmd(siteID string) tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		client, err := GetAPIClientWithSite(ctx, siteID)
		if err != nil {
			return initTeaErrMsg{err: err}
		}
		themes, err := api.List[api.Theme](ctx, client, "/themes")
		if err != nil {
			return initTeaErrMsg{err: fmt.Errorf("list themes: %w", err)}
		}
		return initTeaThemesLoadedMsg{themes: themes}
	}
}

// destinationPath is the directory the project will be written to: the fixed
// positional target when given, otherwise the prompted name under the output dir.
func (m *initTeaModel) destinationPath() string {
	if strings.TrimSpace(m.fixedTarget) != "" {
		return m.fixedTarget
	}
	return filepath.Join(m.outputDir, m.answers.DirectoryName)
}

// selectedThemeLabel returns the human-readable theme label shown on the confirm
// screen, matching the label used in the transcript. It falls back to the
// selection ID when the choice list has not been populated (e.g. in tests).
func (m *initTeaModel) selectedThemeLabel() string {
	for _, choice := range m.prompt.Themes {
		if choice.Theme.ID == m.answers.ThemeID {
			return choice.Label
		}
	}
	return m.answers.ThemeID
}

func (m *initTeaModel) bootstrapOptions() bootstrap.BootstrapOptions {
	// answers.SiteID/ThemeID stay the API IDs used for selection and the
	// init-time API calls; nimbu.yml gets the human-readable subdomain/short.
	site := m.answers.SiteID
	if selected, ok := findSiteByID(m.sites, m.answers.SiteID); ok {
		site = siteConfigValue(selected)
	}
	theme := m.answers.ThemeID
	if selected, ok := findThemeByID(m.themes, m.answers.ThemeID); ok {
		theme = themeConfigValue(selected)
	}
	return bootstrap.BootstrapOptions{
		Manifest:       m.manifest,
		SourceDir:      m.sourceDir,
		DestinationDir: m.destinationPath(),
		Site:           site,
		Theme:          theme,
		BundleIDs:      m.answers.BundleIDs,
		RepeatableIDs:  m.answers.RepeatableIDs,
		AllowExisting:  m.inPlace,
		SkipPaths:      m.skip,
	}
}

func (m *initTeaModel) bootstrapCmd() tea.Cmd {
	opts := m.bootstrapOptions()
	sourceLabel := m.sourceLabel
	return func() tea.Msg {
		result, err := bootstrap.BootstrapProject(opts)
		if err != nil {
			return initTeaErrMsg{err: err}
		}
		return initTeaBootstrapDoneMsg{
			result: initResult{
				Path:        result.Path,
				Site:        result.Site,
				Theme:       result.Theme,
				Source:      sourceLabel,
				Bundles:     result.Bundles,
				Repeatables: result.Repeatables,
			},
		}
	}
}

func siteOptions(sites []initSiteChoice) []initChoiceOption {
	out := make([]initChoiceOption, 0, len(sites))
	for _, site := range sites {
		out = append(out, initChoiceOption{ID: site.Site.ID, Label: site.Label})
	}
	return out
}

func themeOptions(themes []initThemeChoice) []initChoiceOption {
	out := make([]initChoiceOption, 0, len(themes))
	for _, theme := range themes {
		out = append(out, initChoiceOption{ID: theme.Theme.ID, Label: theme.Label})
	}
	return out
}

func repeatableOptions(repeatables []bootstrap.Repeatable) []initChoiceOption {
	out := make([]initChoiceOption, 0, len(repeatables))
	for _, repeatable := range repeatables {
		label := repeatable.Label
		if strings.TrimSpace(label) == "" {
			label = repeatable.ID
		}
		out = append(out, initChoiceOption{ID: repeatable.ID, Label: label})
	}
	return out
}

func bundleOptions(bundles []bootstrap.Bundle) []initChoiceOption {
	out := make([]initChoiceOption, 0, len(bundles))
	for _, bundle := range bundles {
		label := bundle.Label
		if strings.TrimSpace(label) == "" {
			label = bundle.ID
		}
		out = append(out, initChoiceOption{ID: bundle.ID, Label: label})
	}
	return out
}

func sortedSelection(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func joinSelectionsOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func repeatableIDsForWizard(repeatables []bootstrap.Repeatable) []string {
	out := make([]string, 0, len(repeatables))
	for _, repeatable := range repeatables {
		out = append(out, repeatable.ID)
	}
	slices.Sort(out)
	return out
}
