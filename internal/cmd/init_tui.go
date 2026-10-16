package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
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
	initTeaPhaseDone    initTeaPhase = "done"
)

type initTeaStep string

const (
	initTeaStepSite           initTeaStep = "site"
	initTeaStepTheme          initTeaStep = "theme"
	initTeaStepDirectory      initTeaStep = "directory"
	initTeaStepRepeatableMode initTeaStep = "repeatable_mode"
	initTeaStepRepeatables    initTeaStep = "repeatables"
	initTeaStepBundles        initTeaStep = "bundles"
	initTeaStepOverwrite      initTeaStep = "overwrite"
	initTeaStepConfirm        initTeaStep = "confirm"
)

// initSignal records an external signal the wizard received.
type initSignal int

const (
	initSignalNone initSignal = iota
	initSignalInterrupt
	initSignalTerminate
)

// Exit codes for a wizard stopped by a signal (128 + signal number).
const (
	initExitInterrupted = 130 // SIGINT / ctrl+c
	initExitTerminated  = 143 // SIGTERM
)

type initTeaSourceResolvedMsg struct {
	sourceDir   string
	sourceLabel string
	outputDir   string
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

// initTeaHoldMsg replaces a quit or interrupt that arrived while files were
// being written; the wizard finishes the write first.
type initTeaHoldMsg struct{}

type initChoiceOption struct {
	ID    string
	Label string
}

type initTeaModel struct {
	ctx     context.Context
	cmd     *InitCmd
	flags   *RootFlags
	writer  *output.Writer
	work    *initWork
	presets initPresets

	palette  []string
	useColor bool
	isDark   bool
	theme    initTheme

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

	// In-place init state (set when a positional directory argument is given).
	fixedTarget string
	inPlace     bool
	conflicts   []string            // planned files that already exist in the target
	overwrite   map[string]struct{} // checked = overwrite; default all conflicts
	skip        map[string]struct{} // resolved: paths the user declined to overwrite

	// Exit bookkeeping.
	quitting   bool       // the wizard itself asked to quit
	signal     initSignal // external SIGINT/SIGTERM seen by the program
	cancelHeld bool       // a cancel arrived while files were being written
}

func (c *InitCmd) runInteractiveTTY(ctx context.Context, flags *RootFlags) error {
	writer := output.WriterFromContext(ctx)
	model := newInitTeaModel(ctx, c, flags)
	defer model.work.shutdown()

	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(os.Stdin),
		tea.WithOutput(writer.Err),
		tea.WithColorProfile(initColorProfile(writer)),
		tea.WithFilter(initTeaFilter),
		// Bubble Tea's own handler stops listening after the first signal, so
		// a second SIGINT/SIGTERM during the file write would kill the process
		// mid-copy with the terminal still in raw mode. Forward every signal
		// instead; the filter holds them while files are written.
		tea.WithoutSignalHandler(),
	)
	stopSignals := forwardInitSignals(program)
	defer stopSignals()

	finalModel, runErr := program.Run()
	// Stop clones and wait for running jobs before any cleanup or exit.
	// Signals stay caught until then so nothing is left half-written.
	model.work.shutdown()
	stopSignals()

	final, ok := finalModel.(*initTeaModel)
	if !ok || final == nil {
		final = model
	}
	return initTeaOutcome(final, runErr)
}

// initTeaOutcome maps how the program ended to the command result. Only a
// wizard that reached the done phase on its own succeeds; everything else is a
// cancellation with a non-zero exit.
func initTeaOutcome(m *initTeaModel, runErr error) error {
	switch {
	case errors.Is(runErr, tea.ErrInterrupted):
		return &ExitError{Code: initExitInterrupted, Err: initStoppedError(m)}
	case errors.Is(runErr, tea.ErrProgramKilled):
		return fmt.Errorf("%w: %w", errInitCancelled, runErr)
	case runErr != nil:
		return runErr
	case m.err != nil:
		return m.err
	case m.signal == initSignalTerminate:
		return &ExitError{Code: initExitTerminated, Err: initStoppedError(m)}
	case m.signal == initSignalInterrupt:
		return &ExitError{Code: initExitInterrupted, Err: initStoppedError(m)}
	case m.phase != initTeaPhaseDone:
		return errInitCancelled
	}
	return nil
}

func initStoppedError(m *initTeaModel) error {
	if m != nil && m.phase == initTeaPhaseDone && m.result.Path != "" {
		return fmt.Errorf("init interrupted after the project was written to %s", m.result.Path)
	}
	return errInitCancelled
}

// initTeaFilter sees Bubble Tea's QuitMsg/InterruptMsg before the program
// acts on them. SIGTERM arrives as a QuitMsg and SIGINT as an InterruptMsg;
// both are recorded so the command exits non-zero, and while files are being
// written they are held until the write finishes so the project is never left
// half-copied.
func initTeaFilter(model tea.Model, msg tea.Msg) tea.Msg {
	m, ok := model.(*initTeaModel)
	if !ok || m.quitting {
		return msg
	}
	var sig initSignal
	switch msg.(type) {
	case tea.QuitMsg:
		sig = initSignalTerminate
	case tea.InterruptMsg:
		sig = initSignalInterrupt
	default:
		return msg
	}
	if m.signal == initSignalNone {
		m.signal = sig
	}
	if m.phase == initTeaPhaseApply {
		return initTeaHoldMsg{}
	}
	// A quit ends the program gracefully and Bubble Tea renders the model
	// once more: mark it quitting so that last frame says "Cancelled." instead
	// of leaving a live prompt in the scrollback.
	m.quitting = true
	return msg
}

// forwardInitSignals delivers every SIGINT/SIGTERM to the program as Bubble
// Tea's handler would (InterruptMsg/QuitMsg), for as long as the wizard runs.
// The returned stop function releases the signals.
func forwardInitSignals(program *tea.Program) func() {
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case sig := <-sigs:
				// Send is a no-op once the program has exited.
				program.Send(initSignalMsg(sig))
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			signal.Stop(sigs)
			close(done)
		})
	}
}

// initSignalMsg maps a signal to the message Bubble Tea's own handler sends.
func initSignalMsg(sig os.Signal) tea.Msg {
	if sig == os.Interrupt {
		return tea.InterruptMsg{}
	}
	return tea.QuitMsg{}
}

// initColorProfile maps the stderr color decision to the program's profile:
// the wizard draws on stderr, so stdout being piped must not matter.
func initColorProfile(writer *output.Writer) colorprofile.Profile {
	if writer == nil {
		return colorprofile.ASCII
	}
	return teaColorProfile(writer.ErrColorProfile())
}

// teaColorProfile maps a termenv profile (the output package's color
// decision) to the colorprofile the Charm v2 stack renders with.
func teaColorProfile(profile termenv.Profile) colorprofile.Profile {
	switch profile {
	case termenv.TrueColor:
		return colorprofile.TrueColor
	case termenv.ANSI256:
		return colorprofile.ANSI256
	case termenv.ANSI:
		return colorprofile.ANSI
	default:
		return colorprofile.ASCII
	}
}

func newInitTeaModel(ctx context.Context, cmd *InitCmd, flags *RootFlags) *initTeaModel {
	writer := output.WriterFromContext(ctx)
	useColor := output.IsHuman(ctx) && writer != nil && writer.ErrUseColor()
	model := newInitTeaBaseModel(ctx, useColor)
	model.cmd = cmd
	model.flags = flags
	model.writer = writer
	model.presets = cmd.presets(flags)
	if cfg, ok := ctx.Value(configKey{}).(*config.Config); ok && cfg != nil && cfg.BannerTheme != "" {
		if theme, found := BannerThemeByName(cfg.BannerTheme); found {
			model.palette = theme.Palette
		}
	}
	fixedTarget, fixed, ferr := cmd.resolveFixedTarget()
	switch {
	case ferr != nil:
		model.err = ferr
	case fixed:
		model.fixedTarget = fixedTarget
		// A file target (or stat failure) is a clear user error — surface it so
		// the program quits with the precise message instead of a generic one.
		if dir, derr := targetIsExistingDir(true, fixedTarget); derr != nil {
			model.err = derr
		} else {
			model.inPlace = dir
		}
	}
	model.loadingSummary = "Template"
	model.loadingDetail = "Loading template manifest..."
	if strings.TrimSpace(cmd.Dir) == "" {
		model.loadingDetail = "Cloning repository..."
	}
	return model
}

func newInitTeaTestModel(prompt initPromptModel) *initTeaModel {
	model := newInitTeaBaseModel(context.Background(), false)
	model.cmd = &InitCmd{}
	model.phase = initTeaPhasePrompt
	model.prompt = prompt
	model.step = initTeaStepSite
	model.outputDir = prompt.OutputDir
	model.sourceLabel = prompt.Source
	model.answers.DirectoryName = prompt.DefaultDirectoryName
	model.directoryInput.SetValue(prompt.DefaultDirectoryName)
	return model
}

func newInitTeaBaseModel(ctx context.Context, useColor bool) *initTeaModel {
	if ctx == nil {
		ctx = context.Background()
	}
	filterInput := textinput.New()
	filterInput.Prompt = ""
	filterInput.Placeholder = "type to filter"
	filterInput.Focus()

	directoryInput := textinput.New()
	directoryInput.Prompt = ""
	directoryInput.Placeholder = "Directory name"

	model := &initTeaModel{
		ctx:            ctx,
		work:           newInitWork(ctx),
		presets:        initPresets{OnConflict: initConflictAsk},
		palette:        DefaultBannerPalette(),
		useColor:       useColor,
		isDark:         true, // most terminals; upgraded once the background reply arrives
		width:          80,
		height:         24,
		phase:          initTeaPhaseLoading,
		spinner:        spinner.New(spinner.WithSpinner(initSpinner())),
		filterInput:    filterInput,
		directoryInput: directoryInput,
		repeatables:    map[string]struct{}{},
		bundles:        map[string]struct{}{},
		overwrite:      map[string]struct{}{},
	}
	model.applyTheme()
	return model
}

// initSpinner is the Dot spinner without its trailing padding, so the
// spinner takes exactly the marker's single cell on the timeline rail.
func initSpinner() spinner.Spinner {
	frames := make([]string, 0, len(spinner.Dot.Frames))
	for _, frame := range spinner.Dot.Frames {
		frames = append(frames, strings.TrimSpace(frame))
	}
	return spinner.Spinner{Frames: frames, FPS: spinner.Dot.FPS}
}

// applyTheme rebuilds every style from the current color decision and
// background, including the embedded bubbles.
func (m *initTeaModel) applyTheme() {
	m.theme = newInitTheme(m.useColor, m.isDark)
	m.spinner.Style = m.theme.style(m.theme.Active, true)

	styles := textinput.DefaultStyles(m.isDark)
	if !m.useColor {
		plain := lipgloss.NewStyle()
		styles.Focused = textinput.StyleState{Text: plain, Placeholder: plain, Suggestion: plain, Prompt: plain}
		styles.Blurred = styles.Focused
		styles.Cursor.Color = nil
	}
	styles.Cursor.Blink = false
	m.filterInput.SetStyles(styles)
	m.directoryInput.SetStyles(styles)
}

func (m *initTeaModel) Init() tea.Cmd {
	if m.err != nil {
		// A construction-time error (e.g. a file target) — quit immediately so
		// runInteractiveTTY returns the precise message.
		return m.quit()
	}
	cmds := []tea.Cmd{m.spinner.Tick, m.resolveSourceCmd()}
	if m.useColor {
		// Ask for the background so light terminals get the light palette; the
		// dark default renders until (and unless) the terminal answers.
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	return tea.Batch(cmds...)
}

// quit ends the program on the wizard's own terms (done, error or esc).
func (m *initTeaModel) quit() tea.Cmd {
	m.quitting = true
	return tea.Quit
}

func (m *initTeaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.resizeInputs()
		return m, nil
	case tea.BackgroundColorMsg:
		m.isDark = typed.IsDark()
		m.applyTheme()
		return m, nil
	case spinner.TickMsg:
		if m.phase == initTeaPhaseLoading || m.phase == initTeaPhaseApply {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	case initTeaHoldMsg:
		m.cancelHeld = true
		return m, nil
	case initTeaSourceResolvedMsg:
		m.sourceDir = typed.sourceDir
		m.sourceLabel = typed.sourceLabel
		m.outputDir = typed.outputDir
		m.loadingSummary = "Template"
		m.loadingDetail = "Loading template manifest..."
		return m, m.loadManifestCmd()
	case initTeaManifestLoadedMsg:
		return m.onManifestLoaded(typed.manifest)
	case initTeaSitesLoadedMsg:
		return m.onSitesLoaded(typed.sites)
	case initTeaThemesLoadedMsg:
		return m.onThemesLoaded(typed.themes)
	case initTeaBootstrapDoneMsg:
		m.result = typed.result
		m.phase = initTeaPhaseDone
		return m, m.quit()
	case initTeaErrMsg:
		m.err = typed.err
		return m, m.quit()
	case tea.KeyPressMsg:
		return m.handleKey(typed)
	}
	return m, nil
}

func (m *initTeaModel) View() tea.View {
	return tea.NewView(renderInitTea(m))
}

func (m *initTeaModel) onManifestLoaded(manifest bootstrap.Manifest) (tea.Model, tea.Cmd) {
	m.manifest = manifest
	m.prompt.BundleOptions = manifest.Bundles
	m.prompt.RepeatableOptions = manifest.Repeatables
	if m.presets.Repeatables != "" {
		if _, err := parseInitRepeatables(m.presets.Repeatables, manifest.Repeatables); err != nil {
			m.err = &ExitError{Code: ExitUsage, Err: err}
			return m, m.quit()
		}
	}
	if m.presets.Bundles != "" {
		if _, err := parseInitBundles(m.presets.Bundles, manifest.Bundles); err != nil {
			m.err = &ExitError{Code: ExitUsage, Err: err}
			return m, m.quit()
		}
	}
	m.appendTranscript("Template", m.templateDisplayName())
	m.loadingSummary = "Site"
	m.loadingDetail = "Fetching your sites..."
	return m, m.loadSitesCmd()
}

func (m *initTeaModel) onSitesLoaded(sites []api.Site) (tea.Model, tea.Cmd) {
	if len(sites) == 0 {
		m.err = fmt.Errorf("no accessible sites found")
		return m, m.quit()
	}
	m.sites = sites
	m.prompt.Sites = initSiteChoices(sites)
	m.prompt.Source = m.sourceLabel
	m.prompt.OutputDir = m.outputDir
	m.answers = initAnswers{RepeatableMode: "none"}
	if m.presets.Site != "" {
		site, err := matchInitSite(sites, m.presets.Site)
		if err != nil {
			m.err = &ExitError{Code: ExitUsage, Err: err}
			return m, m.quit()
		}
		for _, choice := range m.prompt.Sites {
			if choice.Site.ID == site.ID {
				return m.selectSite(initChoiceOption{ID: site.ID, Label: choice.Label})
			}
		}
	}
	m.enterStep(initTeaStepSite)
	return m, nil
}

func (m *initTeaModel) onThemesLoaded(themes []api.Theme) (tea.Model, tea.Cmd) {
	if len(themes) == 0 {
		m.err = fmt.Errorf("no themes found for site %s", m.answers.SiteID)
		return m, m.quit()
	}
	m.themes = themes
	m.prompt.Themes = initThemeChoices(themes)
	if site, ok := findSiteByID(m.sites, m.answers.SiteID); ok {
		m.prompt.DefaultDirectoryName = "theme-" + parameterize(site.Subdomain)
	}
	m.directoryInput.SetValue(m.prompt.DefaultDirectoryName)
	m.answers.DirectoryName = m.prompt.DefaultDirectoryName

	preset := -1
	switch {
	case m.presets.Theme != "":
		theme, err := matchInitTheme(themes, m.presets.Theme)
		if err != nil {
			m.err = &ExitError{Code: ExitUsage, Err: err}
			return m, m.quit()
		}
		for idx, choice := range m.prompt.Themes {
			if choice.Theme.ID == theme.ID {
				preset = idx
			}
		}
	case len(themes) == 1:
		preset = 0
	}
	if preset >= 0 {
		choice := m.prompt.Themes[preset]
		m.answers.ThemeID = choice.Theme.ID
		m.appendTranscript("Theme", choice.Label)
		m.enterStep(m.stepAfterTheme())
		return m, nil
	}
	m.enterStep(initTeaStepTheme)
	return m, nil
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
