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
	default:
		return nil
	}
}

func (m *initTeaModel) isSelected(id string) bool {
	switch m.step {
	case initTeaStepRepeatables:
		_, ok := m.repeatables[id]
		return ok
	case initTeaStepBundles:
		_, ok := m.bundles[id]
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

func (m *initTeaModel) bootstrapCmd() tea.Cmd {
	manifest := m.manifest
	sourceDir := m.sourceDir
	outputDir := m.outputDir
	answers := m.answers
	return func() tea.Msg {
		finalPath := filepath.Join(outputDir, answers.DirectoryName)
		result, err := bootstrap.BootstrapProject(bootstrap.BootstrapOptions{
			Manifest:       manifest,
			SourceDir:      sourceDir,
			DestinationDir: finalPath,
			Site:           answers.SiteID,
			Theme:          answers.ThemeID,
			BundleIDs:      answers.BundleIDs,
			RepeatableIDs:  answers.RepeatableIDs,
		})
		if err != nil {
			return initTeaErrMsg{err: err}
		}
		return initTeaBootstrapDoneMsg{
			result: initResult{
				Path:        result.Path,
				Site:        result.Site,
				Theme:       result.Theme,
				Source:      m.sourceLabel,
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
