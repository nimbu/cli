package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// handleKey routes a key press by phase. ctrl+c interrupts (exit 130) and esc
// cancels, except while files are being written: then the first press is
// held and the wizard says it is finishing, so the target is never left
// half-written. A held ctrl+c still exits 130 once the write is done, like a
// SIGINT would. A second press stops anyway (exit 130).
func (m *initTeaModel) handleKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case initTeaPhaseApply:
		if isInitCancelKey(key) {
			if m.cancelHeld {
				m.abandonApply()
				return m, tea.Quit
			}
			m.cancelHeld = true
			if key.String() == "ctrl+c" && m.signal == initSignalNone {
				m.signal = initSignalInterrupt
			}
		}
		return m, nil
	case initTeaPhaseDone:
		return m, nil
	case initTeaPhaseLoading:
		return m.handleCancelKey(key)
	}
	if m.tooSmall() {
		return m.handleCancelKey(key)
	}
	switch m.step {
	case initTeaStepDirectory:
		return m.handleDirectoryKey(key)
	case initTeaStepConfirm:
		return m.handleConfirmKey(key)
	default:
		return m.handleListKey(key)
	}
}

func isInitCancelKey(key tea.KeyPressMsg) bool {
	switch key.String() {
	case "ctrl+c", "esc":
		return true
	}
	return false
}

// handleCancelKey handles the two ways out shared by every prompt. ctrl+c is
// recorded as an interrupt (exit 130) but still quits through tea.Quit so the
// final "Cancelled." frame is drawn.
func (m *initTeaModel) handleCancelKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c":
		m.signal = initSignalInterrupt
		return m, m.quit()
	case "esc":
		m.err = errInitCancelled
		return m, m.quit()
	}
	return m, nil
}

func (m *initTeaModel) handleDirectoryKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "esc":
		return m.handleCancelKey(key)
	case "enter":
		value := strings.TrimSpace(m.directoryInput.Value())
		if value == "" {
			value = m.prompt.DefaultDirectoryName
		}
		m.answers.DirectoryName = value
		m.directoryInput.SetValue(value)
		m.appendTranscript("Directory", value)
		m.enterStep(m.nextStepAfterDirectory())
		return m, nil
	}
	var cmd tea.Cmd
	m.directoryInput, cmd = m.directoryInput.Update(key)
	m.answers.DirectoryName = m.directoryInput.Value()
	return m, cmd
}

func (m *initTeaModel) handleConfirmKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "esc":
		return m.handleCancelKey(key)
	case "enter":
		m.phase = initTeaPhaseApply
		m.loadingSummary = "Creating project"
		m.loadingDetail = "Writing files..."
		return m, tea.Batch(m.spinner.Tick, m.bootstrapCmd())
	}
	return m, nil
}

func (m *initTeaModel) isFilterableStep() bool {
	return m.step == initTeaStepSite || m.step == initTeaStepTheme || m.step == initTeaStepRepeatables
}

func (m *initTeaModel) filterActive() bool {
	return m.isFilterableStep() && m.filterInput.Value() != ""
}

func (m *initTeaModel) handleListKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	filterable := m.isFilterableStep()

	switch key.String() {
	case "ctrl+c":
		return m.handleCancelKey(key)
	case "esc":
		// Esc backs out one level: first the filter, then the wizard.
		if m.filterActive() {
			m.filterInput.SetValue("")
			m.clampCursor()
			return m, nil
		}
		return m.handleCancelKey(key)
	case "up":
		m.moveCursor(-1)
		return m, nil
	case "down":
		m.moveCursor(1)
		return m, nil
	case "k":
		if !filterable {
			m.moveCursor(-1)
			return m, nil
		}
	case "j":
		if !filterable {
			m.moveCursor(1)
			return m, nil
		}
	case "space":
		if m.isMultiSelectStep() {
			m.toggleCurrentOption()
			return m, nil
		}
	case "enter":
		return m.confirmCurrentSelection()
	}

	// Delegate remaining keys to filter input
	if filterable {
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
		return m.selectSite(current)
	case initTeaStepTheme:
		m.answers.ThemeID = current.ID
		m.appendTranscript("Theme", current.Label)
		m.enterStep(m.stepAfterTheme())
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
		m.enterStep(m.confirmStep())
	case initTeaStepOverwrite:
		m.resolveOverwriteSelection()
		m.enterStep(initTeaStepConfirm)
	}

	return m, nil
}

func (m *initTeaModel) selectSite(choice initChoiceOption) (tea.Model, tea.Cmd) {
	m.answers.SiteID = choice.ID
	m.appendTranscript("Site", choice.Label)
	m.phase = initTeaPhaseLoading
	m.loadingSummary = "Theme"
	m.loadingDetail = "Fetching themes for your site..."
	return m, tea.Batch(m.spinner.Tick, m.loadThemesCmd(choice.ID))
}

// stepAfterTheme skips the directory-name prompt when a positional target was
// given — the destination path is already fixed.
func (m *initTeaModel) stepAfterTheme() initTeaStep {
	if strings.TrimSpace(m.fixedTarget) != "" {
		return m.nextStepAfterDirectory()
	}
	return initTeaStepDirectory
}

// confirmStep routes to the overwrite checklist when initializing in place and
// planned files already exist; otherwise straight to the final confirmation.
// PlanPaths failures are deferred to bootstrap, which surfaces them as an error.
func (m *initTeaModel) confirmStep() initTeaStep {
	if !m.inPlace {
		return initTeaStepConfirm
	}
	conflicts, err := planConflicts(m.bootstrapOptions(), m.fixedTarget)
	if err != nil || len(conflicts) == 0 {
		return initTeaStepConfirm
	}
	m.conflicts = conflicts
	if m.presets.OnConflict == initConflictOverwrite || m.presets.OnConflict == initConflictKeep {
		m.skip = conflictSkipSet(m.presets.OnConflict, conflicts)
		m.appendTranscript("Overwrite", fmt.Sprintf("%d replaced, %d kept", len(conflicts)-len(m.skip), len(m.skip)))
		return initTeaStepConfirm
	}
	m.overwrite = make(map[string]struct{}, len(conflicts))
	for _, rel := range conflicts {
		m.overwrite[rel] = struct{}{} // default: overwrite all
	}
	return initTeaStepOverwrite
}

// resolveOverwriteSelection converts the checklist selection into the skip set
// (conflicts left unchecked are kept as-is).
func (m *initTeaModel) resolveOverwriteSelection() {
	skip := make(map[string]struct{})
	for _, rel := range m.conflicts {
		if _, overwrite := m.overwrite[rel]; !overwrite {
			skip[rel] = struct{}{}
		}
	}
	m.skip = skip
	m.appendTranscript("Overwrite", fmt.Sprintf("%d replaced, %d kept", len(m.conflicts)-len(skip), len(skip)))
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
	case initTeaStepOverwrite:
		return m.overwrite
	default:
		return map[string]struct{}{}
	}
}

func (m *initTeaModel) isMultiSelectStep() bool {
	return m.step == initTeaStepRepeatables || m.step == initTeaStepBundles || m.step == initTeaStepOverwrite
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

// nextStepAfterDirectory picks the repeatables step, or applies the
// --repeatables preset (or the manifest having none) and moves on.
func (m *initTeaModel) nextStepAfterDirectory() initTeaStep {
	if len(m.prompt.RepeatableOptions) == 0 {
		return m.nextStepAfterRepeatables()
	}
	if m.presets.Repeatables != "" {
		// Validated when the manifest loaded.
		ids, _ := parseInitRepeatables(m.presets.Repeatables, m.prompt.RepeatableOptions)
		m.answers.RepeatableIDs = ids
		m.appendTranscript("Repeatables", joinSelectionsOrNone(ids))
		return m.nextStepAfterRepeatables()
	}
	return initTeaStepRepeatableMode
}

func (m *initTeaModel) nextStepAfterRepeatables() initTeaStep {
	if len(m.prompt.BundleOptions) == 0 {
		return m.confirmStep()
	}
	if m.presets.Bundles != "" {
		ids, _ := parseInitBundles(m.presets.Bundles, m.prompt.BundleOptions)
		m.answers.BundleIDs = ids
		m.appendTranscript("Bundles", joinSelectionsOrNone(ids))
		return m.confirmStep()
	}
	return initTeaStepBundles
}

func (m *initTeaModel) resizeInputs() {
	fieldWidth := max(m.width-16, 8)
	m.filterInput.SetWidth(fieldWidth)
	m.directoryInput.SetWidth(fieldWidth)
}
