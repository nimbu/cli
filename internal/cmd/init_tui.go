package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/bootstrap"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

type initTranscriptEntry struct {
	Label string
	Value string
	Text  string
}

type initTeaPhase string

const (
	initTeaPhaseLoading initTeaPhase = "loading"
	initTeaPhasePrompt  initTeaPhase = "prompt"
	initTeaPhaseApply   initTeaPhase = "apply"
)

type initTeaStep string

const (
	initTeaStepSite           initTeaStep = "site"
	initTeaStepTheme          initTeaStep = "theme"
	initTeaStepDirectory      initTeaStep = "directory"
	initTeaStepRepeatableMode initTeaStep = "repeatable_mode"
	initTeaStepRepeatables    initTeaStep = "repeatables"
	initTeaStepBundles        initTeaStep = "bundles"
	initTeaStepConfirm        initTeaStep = "confirm"
)

type initTeaSourceResolvedMsg struct {
	sourceDir   string
	sourceLabel string
	outputDir   string
	cleanup     func()
}

type initTeaManifestLoadedMsg struct {
	manifest bootstrap.Manifest
}

type initTeaSitesLoadedMsg struct {
	sites []api.Site
}

type initTeaThemesLoadedMsg struct {
	themes []api.Theme
}

type initTeaBootstrapDoneMsg struct {
	result initResult
}

type initTeaErrMsg struct {
	err error
}

type initChoiceOption struct {
	ID    string
	Label string
}

type initTeaModel struct {
	ctx      context.Context
	cmd      *InitCmd
	flags    *RootFlags
	writer   *output.Writer
	renderer *lipgloss.Renderer
	palette  []string
	useColor bool

	width  int
	height int

	phase          initTeaPhase
	step           initTeaStep
	loadingSummary string
	loadingDetail  string

	spinner        spinner.Model
	filterInput    textinput.Model
	directoryInput textinput.Model

	manifest    bootstrap.Manifest
	sourceDir   string
	sourceLabel string
	outputDir   string
	cleanup     func()

	sites  []api.Site
	themes []api.Theme
	prompt initPromptModel

	answers     initAnswers
	transcript  []initTranscriptEntry
	cursor      int
	repeatables map[string]struct{}
	bundles     map[string]struct{}
	err         error
	result      initResult
}

func (c *InitCmd) runInteractiveTTY(ctx context.Context, flags *RootFlags) error {
	writer := output.WriterFromContext(ctx)
	program := tea.NewProgram(
		newInitTeaModel(ctx, c, flags),
		tea.WithContext(ctx),
		tea.WithInput(os.Stdin),
		tea.WithOutput(writer.Err),
	)

	finalModel, err := program.Run()
	if err != nil {
		return err
	}

	model, ok := finalModel.(*initTeaModel)
	if !ok {
		return fmt.Errorf("unexpected init ui model type %T", finalModel)
	}
	if model.cleanup != nil {
		defer model.cleanup()
	}
	if model.err != nil {
		return model.err
	}
	return emitInitResult(ctx, model.result)
}

func newInitTeaModel(ctx context.Context, cmd *InitCmd, flags *RootFlags) *initTeaModel {
	writer := output.WriterFromContext(ctx)
	useColor := output.IsHuman(ctx) && writer.UseColor()
	model := newInitTeaBaseModel(useColor, initTeaRendererForWriter(writer, useColor))
	model.ctx = ctx
	model.cmd = cmd
	model.flags = flags
	model.writer = writer
	model.useColor = useColor
	model.palette = DefaultBannerPalette()
	if cfg, ok := ctx.Value(configKey{}).(*config.Config); ok && cfg != nil && cfg.BannerTheme != "" {
		if theme, found := BannerThemeByName(cfg.BannerTheme); found {
			model.palette = theme.Palette
		}
	}
	if strings.TrimSpace(cmd.Dir) == "" {
		model.loadingSummary = "Template"
		model.loadingDetail = "Cloning repository..."
	} else {
		model.loadingSummary = "Template"
		model.loadingDetail = "Loading template manifest..."
	}
	return model
}

func newInitTeaTestModel(prompt initPromptModel) *initTeaModel {
	model := newInitTeaBaseModel(false, initTeaRendererForWriter(&output.Writer{Err: io.Discard, Color: "never"}, false))
	model.phase = initTeaPhasePrompt
	model.prompt = prompt
	model.step = initTeaStepSite
	model.palette = DefaultBannerPalette()
	model.outputDir = prompt.OutputDir
	model.sourceLabel = prompt.Source
	model.answers.DirectoryName = prompt.DefaultDirectoryName
	model.directoryInput.SetValue(prompt.DefaultDirectoryName)
	return model
}

func newInitTeaBaseModel(useColor bool, renderer *lipgloss.Renderer) *initTeaModel {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = initTeaAccentStyle(useColor)

	filterInput := textinput.New()
	filterInput.Prompt = ""
	filterInput.Placeholder = "Type to filter"
	filterInput.Focus()

	directoryInput := textinput.New()
	directoryInput.Prompt = ""
	directoryInput.Placeholder = "Directory name"

	return &initTeaModel{
		width:          84,
		height:         24,
		phase:          initTeaPhaseLoading,
		spinner:        spin,
		filterInput:    filterInput,
		directoryInput: directoryInput,
		renderer:       renderer,
		repeatables:    map[string]struct{}{},
		bundles:        map[string]struct{}{},
	}
}

func initTeaRendererForWriter(writer *output.Writer, useColor bool) *lipgloss.Renderer {
	out := io.Discard
	if writer != nil && writer.Err != nil {
		out = writer.Err
	}

	renderer := lipgloss.NewRenderer(out)
	if !useColor {
		renderer.SetColorProfile(termenv.Ascii)
		return renderer
	}

	switch writer.Color {
	case "always":
		renderer.SetColorProfile(termenv.TrueColor)
	case "never":
		renderer.SetColorProfile(termenv.Ascii)
	default:
		renderer.SetColorProfile(termenv.EnvColorProfile())
	}

	return renderer
}

func (m *initTeaModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.resolveSourceCmd())
}

func (m *initTeaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(typed.Width, 40)
		m.height = max(typed.Height, 16)
		m.resizeInputs()
		return m, nil
	case spinner.TickMsg:
		if m.phase == initTeaPhaseLoading || m.phase == initTeaPhaseApply {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case initTeaSourceResolvedMsg:
		m.sourceDir = typed.sourceDir
		m.sourceLabel = typed.sourceLabel
		m.outputDir = typed.outputDir
		m.cleanup = typed.cleanup
		m.loadingSummary = "Template"
		m.loadingDetail = "Loading template manifest..."
		return m, m.loadManifestCmd()
	case initTeaManifestLoadedMsg:
		m.manifest = typed.manifest
		m.prompt.BundleOptions = typed.manifest.Bundles
		m.prompt.RepeatableOptions = typed.manifest.Repeatables
		m.appendTranscript("Template", m.templateDisplayName())
		m.loadingSummary = "Site"
		m.loadingDetail = "Fetching your sites..."
		return m, m.loadSitesCmd()
	case initTeaSitesLoadedMsg:
		if len(typed.sites) == 0 {
			m.err = fmt.Errorf("no accessible sites found")
			return m, tea.Quit
		}
		m.sites = typed.sites
		m.prompt.Sites = initSiteChoices(typed.sites)
		m.prompt.Source = m.sourceLabel
		m.prompt.OutputDir = m.outputDir
		m.answers = initAnswers{RepeatableMode: "none"}
		m.enterStep(initTeaStepSite)
		m.phase = initTeaPhasePrompt
		return m, nil
	case initTeaThemesLoadedMsg:
		if len(typed.themes) == 0 {
			m.err = fmt.Errorf("no themes found for site %s", m.answers.SiteID)
			return m, tea.Quit
		}
		m.themes = typed.themes
		m.prompt.Themes = initThemeChoices(typed.themes)
		if site, ok := findSiteByID(m.sites, m.answers.SiteID); ok {
			m.prompt.DefaultDirectoryName = "theme-" + parameterize(site.Subdomain)
		}
		m.directoryInput.SetValue(m.prompt.DefaultDirectoryName)
		m.answers.DirectoryName = m.prompt.DefaultDirectoryName
		if len(typed.themes) == 1 {
			choice := m.prompt.Themes[0]
			m.answers.ThemeID = choice.Theme.ID
			m.appendTranscript("Theme", choice.Label)
			m.phase = initTeaPhasePrompt
			m.enterStep(initTeaStepDirectory)
			return m, nil
		}
		m.phase = initTeaPhasePrompt
		m.enterStep(initTeaStepTheme)
		return m, nil
	case initTeaBootstrapDoneMsg:
		m.result = typed.result
		return m, tea.Quit
	case initTeaErrMsg:
		m.err = typed.err
		return m, tea.Quit
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch m.phase {
		case initTeaPhaseLoading, initTeaPhaseApply:
			if key.String() == "ctrl+c" || key.String() == "esc" {
				m.err = fmt.Errorf("init cancelled")
				return m, tea.Quit
			}
			return m, nil
		case initTeaPhasePrompt:
			return m.handlePromptKey(key)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *initTeaModel) View() string {
	return renderInitTea(m)
}

func (m *initTeaModel) handlePromptKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case initTeaStepDirectory:
		return m.handleDirectoryKey(key)
	case initTeaStepConfirm:
		return m.handleConfirmKey(key)
	default:
		return m.handleListKey(key)
	}
}

func (m *initTeaModel) handleDirectoryKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "esc":
		m.err = fmt.Errorf("init cancelled")
		return m, tea.Quit
	case "enter":
		value := strings.TrimSpace(m.directoryInput.Value())
		if value == "" {
			value = m.prompt.DefaultDirectoryName
		}
		m.answers.DirectoryName = value
		m.directoryInput.SetValue(value)
		m.appendTranscript("Directory", value)
		m.enterStep(m.nextStepAfterDirectory())
	default:
		var cmd tea.Cmd
		m.directoryInput, cmd = m.directoryInput.Update(key)
		m.answers.DirectoryName = m.directoryInput.Value()
		return m, cmd
	}
	return m, nil
}

func (m *initTeaModel) handleConfirmKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "esc":
		m.err = fmt.Errorf("init cancelled")
		return m, tea.Quit
	case "enter":
		m.phase = initTeaPhaseApply
		m.loadingSummary = "Creating project"
		m.loadingDetail = "Creating your project..."
		return m, tea.Batch(m.spinner.Tick, m.bootstrapCmd())
	}
	return m, nil
}

func (m *initTeaModel) handleListKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "esc":
		m.err = fmt.Errorf("init cancelled")
		return m, tea.Quit
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case " ":
		if m.isMultiSelectStep() {
			m.toggleCurrentOption()
		}
	case "enter":
		return m.confirmCurrentSelection()
	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(key)
		m.clampCursor()
		return m, cmd
	}
	return m, nil
}

func (m *initTeaModel) confirmCurrentSelection() (tea.Model, tea.Cmd) {
	options := m.filteredOptions()
	if len(options) == 0 {
		return m, nil
	}
	current := options[m.cursor]

	switch m.step {
	case initTeaStepSite:
		m.answers.SiteID = current.ID
		m.appendTranscript("Site", current.Label)
		m.phase = initTeaPhaseLoading
		m.loadingSummary = "Theme"
		m.loadingDetail = "Fetching themes for your site..."
		return m, tea.Batch(m.spinner.Tick, m.loadThemesCmd(current.ID))
	case initTeaStepTheme:
		m.answers.ThemeID = current.ID
		m.appendTranscript("Theme", current.Label)
		m.enterStep(initTeaStepDirectory)
	case initTeaStepRepeatableMode:
		m.answers.RepeatableMode = current.ID
		switch current.ID {
		case "all":
			m.answers.RepeatableIDs = repeatableIDsForWizard(m.prompt.RepeatableOptions)
			m.appendTranscript("Repeatables", "all")
			m.enterStep(m.nextStepAfterRepeatables())
		case "select":
			m.enterStep(initTeaStepRepeatables)
		default:
			m.answers.RepeatableIDs = nil
			m.repeatables = map[string]struct{}{}
			m.appendTranscript("Repeatables", "none")
			m.enterStep(m.nextStepAfterRepeatables())
		}
	case initTeaStepRepeatables:
		m.answers.RepeatableIDs = sortedSelection(m.repeatables)
		m.appendTranscript("Repeatables", joinSelectionsOrNone(m.answers.RepeatableIDs))
		m.enterStep(m.nextStepAfterRepeatables())
	case initTeaStepBundles:
		m.answers.BundleIDs = sortedSelection(m.bundles)
		m.appendTranscript("Bundles", joinSelectionsOrNone(m.answers.BundleIDs))
		m.enterStep(initTeaStepConfirm)
	}

	return m, nil
}

func (m *initTeaModel) moveCursor(delta int) {
	options := m.filteredOptions()
	if len(options) == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = len(options) - 1
	}
	if m.cursor >= len(options) {
		m.cursor = 0
	}
}

func (m *initTeaModel) clampCursor() {
	options := m.filteredOptions()
	if len(options) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(options) {
		m.cursor = len(options) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *initTeaModel) toggleCurrentOption() {
	options := m.filteredOptions()
	if len(options) == 0 {
		return
	}
	id := options[m.cursor].ID
	target := m.selectionTarget()
	if _, ok := target[id]; ok {
		delete(target, id)
		return
	}
	target[id] = struct{}{}
}

func (m *initTeaModel) selectionTarget() map[string]struct{} {
	switch m.step {
	case initTeaStepRepeatables:
		return m.repeatables
	case initTeaStepBundles:
		return m.bundles
	default:
		return map[string]struct{}{}
	}
}

func (m *initTeaModel) isMultiSelectStep() bool {
	return m.step == initTeaStepRepeatables || m.step == initTeaStepBundles
}

func (m *initTeaModel) enterStep(step initTeaStep) {
	m.phase = initTeaPhasePrompt
	m.step = step
	m.cursor = 0
	m.filterInput.SetValue("")
	m.filterInput.CursorEnd()
	m.filterInput.Focus()
	m.resizeInputs()
	if step == initTeaStepDirectory {
		m.directoryInput.SetValue(defaultString(m.answers.DirectoryName, m.prompt.DefaultDirectoryName))
		m.directoryInput.CursorEnd()
		m.directoryInput.Focus()
	}
	if step == initTeaStepConfirm && len(m.prompt.BundleOptions) == 0 {
		m.answers.BundleIDs = nil
	}
}

func (m *initTeaModel) nextStepAfterDirectory() initTeaStep {
	if len(m.prompt.RepeatableOptions) > 0 {
		return initTeaStepRepeatableMode
	}
	if len(m.prompt.BundleOptions) > 0 {
		return initTeaStepBundles
	}
	return initTeaStepConfirm
}

func (m *initTeaModel) nextStepAfterRepeatables() initTeaStep {
	if len(m.prompt.BundleOptions) > 0 {
		return initTeaStepBundles
	}
	return initTeaStepConfirm
}

func (m *initTeaModel) resizeInputs() {
	bodyWidth := initTeaBodyWidth(m.width)
	fieldWidth := max(bodyWidth-14, 12)
	m.filterInput.Width = fieldWidth
	m.directoryInput.Width = fieldWidth
}

func (m *initTeaModel) appendTranscript(label, value string) {
	if len(m.transcript) > 0 && m.transcript[len(m.transcript)-1].Label == label {
		m.transcript[len(m.transcript)-1].Value = value
		m.transcript[len(m.transcript)-1].Text = ""
		return
	}
	m.transcript = append(m.transcript, initTranscriptEntry{Label: label, Value: value})
}

func (m *initTeaModel) templateDisplayName() string {
	if name := strings.TrimSpace(m.manifest.Name); name != "" {
		return name
	}

	source := strings.TrimSpace(m.sourceLabel)
	if source == "" {
		return "Template"
	}

	source = strings.TrimSuffix(source, ".git")
	if strings.Contains(source, "@") && !filepath.IsAbs(source) && !strings.HasPrefix(source, ".") {
		return strings.Replace(source, "@", "#", 1)
	}

	base := filepath.Base(source)
	if strings.TrimSpace(base) == "" || base == "." || base == string(filepath.Separator) {
		return "Template"
	}
	return base
}

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
