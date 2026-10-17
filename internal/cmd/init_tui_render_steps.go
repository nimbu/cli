package cmd

import (
	"fmt"
	"strings"
)

// activeTimelineLines renders the current step: a spinner row while loading or
// writing, the prompt otherwise. listRows is how many option rows fit.
func activeTimelineLines(m *initTeaModel, width, listRows int) []string {
	switch m.phase {
	case initTeaPhaseLoading, initTeaPhaseApply:
		lines := []string{renderLabelValueLine(m, m.loadingSummary, m.loadingDetail, width)}
		if strings.TrimSpace(m.loadingDetail) == "" {
			lines = []string{m.theme.value(truncateInit(m.loadingSummary, width))}
		}
		switch {
		case m.phase == initTeaPhaseLoading:
			lines = append(lines, initHint(m, width))
		case m.cancelHeld:
			lines = append(lines,
				m.theme.warn(truncateInit("Finishing: stopping now would leave a half-written project.", width)),
				m.theme.muted(truncateInit("Ctrl-C again to stop anyway", width)))
		}
		return lines
	case initTeaPhasePrompt:
		return promptTimelineLines(m, width, listRows)
	default:
		return nil
	}
}

func promptTimelineLines(m *initTeaModel, width, listRows int) []string {
	switch m.step {
	case initTeaStepSite:
		lines := []string{renderLabelValueLine(m, "Site", "choose a site", width), renderSearchLine(m, width)}
		lines = append(lines, optionRows(m, width, listRows)...)
		return append(lines, initHint(m, width, "↑↓ move", "enter select"))
	case initTeaStepTheme:
		lines := []string{renderLabelValueLine(m, "Theme", "choose a theme", width), renderSearchLine(m, width)}
		lines = append(lines, optionRows(m, width, listRows)...)
		return append(lines, initHint(m, width, "↑↓ move", "enter select"))
	case initTeaStepRepeatableMode:
		lines := []string{renderLabelValueLine(m, "Repeatables", "which to copy?", width)}
		lines = append(lines, optionRows(m, width, listRows)...)
		return append(lines, initHint(m, width, "↑↓ move", "enter select"))
	case initTeaStepRepeatables:
		lines := []string{renderLabelValueLine(m, "Repeatables", "choose repeatables", width), renderSearchLine(m, width)}
		lines = append(lines, optionRows(m, width, listRows)...)
		lines = append(lines, renderLabelValueLine(m, "Selected", joinSelectionsOrNone(sortedSelection(m.repeatables)), width))
		return append(lines, initHint(m, width, "↑↓ move", "space toggle", "enter confirm"))
	case initTeaStepBundles:
		lines := []string{renderLabelValueLine(m, "Functional areas", "choose what to include", width)}
		lines = append(lines, optionRows(m, width, listRows)...)
		lines = append(lines, renderLabelValueLine(m, "Selected", joinSelectionsOrNone(sortedSelection(m.bundles)), width))
		return append(lines, initHint(m, width, "↑↓ move", "space toggle", "enter confirm"))
	case initTeaStepDirectory:
		return []string{
			renderLabelLine(m, "Directory", m.directoryInput.View(), width),
			initHint(m, width, "type to edit", "enter confirm"),
		}
	case initTeaStepOverwrite:
		lines := []string{renderLabelValueLine(m, "Overwrite", "checked files are replaced", width)}
		lines = append(lines, optionRows(m, width, listRows)...)
		return append(lines, initHint(m, width, "↑↓ move", "space toggle", "enter confirm"))
	case initTeaStepConfirm:
		lines := []string{
			renderLabelValueLine(m, "Creating project", "review your settings", width),
			renderLabelValueLine(m, "Path", m.destinationPath(), width),
			renderLabelValueLine(m, "Theme", m.selectedThemeLabel(), width),
		}
		if len(m.answers.RepeatableIDs) > 0 {
			lines = append(lines, renderLabelValueLine(m, "Repeatables", strings.Join(m.answers.RepeatableIDs, ", "), width))
		}
		if len(m.answers.BundleIDs) > 0 {
			lines = append(lines, renderLabelValueLine(m, "Bundles", strings.Join(m.answers.BundleIDs, ", "), width))
		}
		lines = append(lines, "")
		return append(lines, m.theme.active(truncateInit("enter create project · esc cancel", width)))
	default:
		return nil
	}
}

// initHint renders the key hint line. Esc is always listed: it clears an
// active filter first, and cancels the wizard otherwise.
func initHint(m *initTeaModel, width int, keys ...string) string {
	esc := "esc cancel"
	if m.filterActive() {
		esc = "esc clear filter"
	}
	return m.theme.muted(truncateInit(strings.Join(append(keys, esc), " · "), width))
}

func renderSearchLine(m *initTeaModel, width int) string {
	query := m.filterInput.Value()
	if query == "" {
		return m.theme.muted(truncateInit("Search: type to filter", width))
	}
	return renderLabelValueLine(m, "Search", query, width)
}

// optionRows renders at most rows lines of the active list, with "↑ N more" /
// "↓ N more" lines when options are cut off above or below the cursor.
func optionRows(m *initTeaModel, width, rows int) []string {
	options := m.filteredOptions()
	if len(options) == 0 {
		return []string{m.theme.muted("No matches")}
	}
	if rows <= 0 {
		return nil
	}
	start, end := initListWindow(len(options), m.cursor, rows)

	lines := make([]string, 0, rows)
	if start > 0 {
		lines = append(lines, m.theme.muted(fmt.Sprintf("↑ %d more", start)))
	}
	for idx := start; idx < end; idx++ {
		option := options[idx]
		prefix := "  "
		if idx == m.cursor {
			prefix = m.theme.active("›") + " "
		}
		if m.isMultiSelectStep() {
			if m.isSelected(option.ID) {
				prefix += m.theme.active("●") + " "
			} else {
				prefix += "○ "
			}
		}
		label := option.Label
		if idx == m.cursor {
			label = m.theme.value(label)
		}
		lines = append(lines, truncateInit(prefix+label, width))
	}
	if end < len(options) {
		lines = append(lines, m.theme.muted(fmt.Sprintf("↓ %d more", len(options)-end)))
	}
	return lines
}

// initListWindow picks the visible slice [start, end) of a list of total items
// so the cursor stays visible and, counting the "more" indicator lines, the
// list uses at most rows lines.
func initListWindow(total, cursor, rows int) (int, int) {
	if total <= rows {
		return 0, total
	}
	if rows < 3 {
		start := min(max(cursor-rows/2, 0), total-rows)
		return start, start + rows
	}
	// Near the top only "↓ more" is needed; near the bottom only "↑ more".
	if cursor < rows-1 {
		return 0, rows - 1
	}
	if cursor >= total-(rows-1) {
		return total - (rows - 1), total
	}
	slots := rows - 2
	start := min(max(cursor-slots/2, 1), total-slots-1)
	return start, start + slots
}
