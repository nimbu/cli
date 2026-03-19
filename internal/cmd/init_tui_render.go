package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

func renderInitTea(model *initTeaModel) string {
	width := max(model.width, 48)

	sections := []string{
		renderInitTeaHeader(model, width),
		"",
		renderInitTeaIntro(model, width),
		renderTimelineConnector(model),
		renderInitTeaTimeline(model, width),
	}

	return strings.Join(sections, "\n")
}

func renderInitTeaIntro(model *initTeaModel, width int) string {
	plain := "┌ Let's setup a new project."
	if !model.useColor {
		return truncateRunes(plain, width)
	}

	corner := initTeaDimStyle(model).Render("┌")
	text := initTeaBrightText(model, "Let's setup a new project.")
	return truncateRunes(corner+" "+text, width)
}

func renderInitTeaHeader(model *initTeaModel, width int) string {
	useColor := model.useColor
	palette := model.palette
	lines := strings.Split(strings.Trim(serverBanner, "\n"), "\n")
	maxLine := 0
	for _, line := range lines {
		maxLine = max(maxLine, utf8.RuneCountInString(line))
	}

	if width < maxLine+6 {
		title := "Nimbu init"
		style := initTeaStyle(model).Bold(true)
		if useColor {
			style = style.Foreground(lipgloss.Color("#22c55e"))
		}
		return style.Width(width).Render(title)
	}

	border := initTeaStyle(model)
	if useColor {
		border = border.Foreground(lipgloss.Color("#64748b"))
	}
	body := initTeaStyle(model)
	if useColor {
		body = body.Foreground(lipgloss.Color("#e2e8f0"))
	}

	var bannerRows []string
	label := " nimbu "
	frameWidth := max(maxLine+4, utf8.RuneCountInString(label)+2)
	top := framedBannerEdge(frameWidth-2, label)
	bannerRows = append(bannerRows, border.Render(top))
	bannerRows = append(bannerRows, border.Render("|"+strings.Repeat(" ", frameWidth)+"|"))
	for idx, line := range lines {
		padding := strings.Repeat(" ", maxLine-utf8.RuneCountInString(line))
		row := "|  " + line + padding + "  |"
		if useColor {
			colorLine := body
			if len(palette) > 0 {
				colorLine = colorLine.Foreground(lipgloss.Color(palette[idx%len(palette)]))
			}
			row = border.Render("|  ") + colorLine.Render(line+padding) + border.Render("  |")
		}
		bannerRows = append(bannerRows, row)
	}
	bannerRows = append(bannerRows, border.Render("+"+strings.Repeat("-", frameWidth)+"+"))

	return initTeaStyle(model).MaxWidth(width).Render(strings.Join(bannerRows, "\n"))
}

func renderInitTeaTimeline(model *initTeaModel, width int) string {
	contentWidth := max(width-8, 28)
	entries := make([]string, 0, len(model.transcript)*2+8)

	for _, entry := range model.transcript {
		lines := transcriptEntryLines(model, entry, contentWidth)
		entries = append(entries, renderTimelineEntry(model, initTimelineDone, lines)...)
		entries = append(entries, renderTimelineConnector(model))
	}

	activeLines := activeTimelineLines(model, contentWidth)
	if len(activeLines) > 0 {
		entries = append(entries, renderTimelineEntry(model, initTimelineActive, activeLines)...)
	}

	// Bubble Tea v1's inline renderer erases the last line on shutdown
	// (EraseEntireLine + \r in standardRenderer.stop). In the done phase,
	// keep the trailing connector as a sacrificial line that gets erased.
	// The durable footer is printed after program.Run() returns.
	if model.phase != initTeaPhaseDone {
		for len(entries) > 0 && entries[len(entries)-1] == renderTimelineConnector(model) {
			entries = entries[:len(entries)-1]
		}
	}

	return strings.Join(entries, "\n")
}

type initTimelineState string

const (
	initTimelineDone   initTimelineState = "done"
	initTimelineActive initTimelineState = "active"
)

func renderTimelineEntry(model *initTeaModel, state initTimelineState, lines []string) []string {
	if len(lines) == 0 {
		return nil
	}

	marker := "◇"
	if state == initTimelineActive {
		marker = "●"
		if model.phase == initTeaPhaseLoading || model.phase == initTeaPhaseApply {
			marker = model.spinner.View()
		}
	}

	rows := make([]string, 0, len(lines))
	for idx, line := range lines {
		rail := renderTimelineRail(model, marker, state)
		if idx > 0 {
			rail = renderTimelineRail(model, "│", "")
		}
		rows = append(rows, rail+" "+line)
	}
	return rows
}

func renderTimelineConnector(model *initTeaModel) string {
	return renderTimelineRail(model, "│", "")
}

func renderTimelineRail(model *initTeaModel, symbol string, state initTimelineState) string {
	style := initTeaStyle(model)
	switch {
	case !model.useColor:
		return symbol
	case state == initTimelineDone:
		return style.Foreground(lipgloss.Color("#22c55e")).Bold(true).Render(symbol)
	case state == initTimelineActive:
		return style.Foreground(lipgloss.Color("#60a5fa")).Bold(true).Render(symbol)
	default:
		return style.Foreground(lipgloss.Color("#94a3b8")).Render(symbol)
	}
}

func activeTimelineLines(model *initTeaModel, width int) []string {
	switch model.phase {
	case initTeaPhaseLoading, initTeaPhaseApply:
		detail := strings.TrimSpace(model.loadingDetail)
		if detail == "" {
			return []string{truncateRunes(model.loadingSummary, width)}
		}
		return []string{renderLabelValueLine(model, model.loadingSummary, detail, width)}
	case initTeaPhasePrompt:
		return promptTimelineLines(model, width)
	default:
		return nil
	}
}

func renderInitTeaDoneFooter(model *initTeaModel) string {
	corner := "└"
	label := "Done!"
	dirname := filepath.Base(model.result.Path)
	hint := "To start working, run:"
	command := "cd " + dirname
	if install := detectInstallCommand(model.result.Path); install != "" {
		command += " && " + install
	}

	if model.useColor {
		corner = initTeaDimStyle(model).Render(corner)
		label = initTeaStyle(model).Bold(true).Foreground(lipgloss.Color("#22c55e")).Render(label)
		hint = initTeaDimStyle(model).Render(hint)
		command = initTeaBrightText(model, command)
	}
	return corner + " " + label + " " + hint + " " + command
}

func detectInstallCommand(projectPath string) string {
	if _, err := os.Stat(filepath.Join(projectPath, "package.json")); err != nil {
		return ""
	}
	switch {
	case fileExists(filepath.Join(projectPath, "pnpm-lock.yaml")):
		return "pnpm install"
	case fileExists(filepath.Join(projectPath, "yarn.lock")):
		return "yarn"
	case fileExists(filepath.Join(projectPath, "bun.lockb")) || fileExists(filepath.Join(projectPath, "bun.lock")):
		return "bun install"
	default:
		return "npm install"
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func promptTimelineLines(model *initTeaModel, width int) []string {
	switch model.step {
	case initTeaStepSite:
		return append([]string{renderLabelValueLine(model, "Site", "choose a site", width)}, filterableOptionLines(model, width)...)
	case initTeaStepTheme:
		return append([]string{renderLabelValueLine(model, "Theme", "choose a theme", width)}, filterableOptionLines(model, width)...)
	case initTeaStepRepeatableMode:
		lines := []string{renderLabelValueLine(model, "Repeatables", "which to copy?", width)}
		lines = append(lines, optionRows(model, width)...)
		lines = append(lines, dimTimelineText(model, "↑↓ move, Enter confirm"))
		return lines
	case initTeaStepRepeatables:
		lines := []string{renderLabelValueLine(model, "Repeatables", "choose repeatables", width), renderSearchLine(model, width)}
		lines = append(lines, optionRows(model, width)...)
		lines = append(lines, selectedSummaryLine(model, "Selected", sortedSelection(model.repeatables), width))
		lines = append(lines, dimTimelineText(model, "↑↓ move, Space select, Enter confirm"))
		return lines
	case initTeaStepBundles:
		lines := []string{renderLabelValueLine(model, "Functional areas", "choose what to include", width)}
		lines = append(lines, optionRows(model, width)...)
		lines = append(lines, selectedSummaryLine(model, "Selected", sortedSelection(model.bundles), width))
		lines = append(lines, dimTimelineText(model, "↑↓ move, Space select, Enter confirm"))
		return lines
	case initTeaStepDirectory:
		lines := []string{
			renderLabelValueLine(model, "Directory", model.directoryInput.View(), width),
			dimTimelineText(model, "Type to edit, Enter confirm"),
		}
		return lines
	case initTeaStepConfirm:
		lines := []string{
			renderLabelValueLine(model, "Creating project", "review your settings", width),
			renderLabelValueLine(model, "Path", filepath.Join(model.outputDir, model.answers.DirectoryName), width),
			renderLabelValueLine(model, "Theme", model.answers.ThemeID, width),
		}
		if len(model.answers.RepeatableIDs) > 0 {
			lines = append(lines, renderLabelValueLine(model, "Repeatables", strings.Join(model.answers.RepeatableIDs, ", "), width))
		}
		if len(model.answers.BundleIDs) > 0 {
			lines = append(lines, renderLabelValueLine(model, "Bundles", strings.Join(model.answers.BundleIDs, ", "), width))
		}
		lines = append(lines, "")
		lines = append(lines, actionTimelineText(model, "Enter create project, Esc cancel"))
		return lines
	default:
		return nil
	}
}

func filterableOptionLines(model *initTeaModel, width int) []string {
	lines := []string{renderSearchLine(model, width)}
	lines = append(lines, optionRows(model, width)...)
	lines = append(lines, dimTimelineText(model, "↑↓ move, Enter confirm"))
	return lines
}

func renderSearchLine(model *initTeaModel, width int) string {
	query := strings.TrimSpace(model.filterInput.Value())
	if query == "" {
		query = "Type to filter"
	}
	return renderLabelValueLine(model, "Search", query, width)
}

func optionRows(model *initTeaModel, width int) []string {
	options := model.filteredOptions()
	if len(options) == 0 {
		return []string{"No matches"}
	}

	maxVisible := max(model.height-18, 4)
	start := 0
	if len(options) > maxVisible {
		start = max(model.cursor-maxVisible/2, 0)
		if end := start + maxVisible; end > len(options) {
			start = len(options) - maxVisible
		}
	}
	end := min(start+maxVisible, len(options))

	lines := make([]string, 0, end-start)
	for idx := start; idx < end; idx++ {
		option := options[idx]
		prefix := "  "
		if idx == model.cursor {
			prefix = "› "
		}
		if model.isMultiSelectStep() {
			if model.isSelected(option.ID) {
				prefix += renderSelectedOptionMarker(model) + " "
			} else {
				prefix += "○ "
			}
		}
		lines = append(lines, truncateRunes(prefix+option.Label, width))
	}
	return lines
}

func selectedSummaryLine(model *initTeaModel, label string, values []string, width int) string {
	value := joinSelectionsOrNone(values)
	return renderLabelValueLine(model, label, value, width)
}

func transcriptEntryLines(model *initTeaModel, entry initTranscriptEntry, width int) []string {
	if strings.TrimSpace(entry.Text) != "" {
		return wrapTimelineText(entry.Text, width)
	}
	if strings.TrimSpace(entry.Label) == "" {
		return wrapTimelineText(entry.Value, width)
	}
	return []string{renderLabelValueLine(model, entry.Label, entry.Value, width)}
}

func wrapTimelineText(value string, width int) []string {
	if width <= 0 {
		return nil
	}
	if utf8.RuneCountInString(value) <= width {
		return []string{value}
	}
	return []string{truncateRunes(value, width)}
}

func renderLabelValueLine(model *initTeaModel, label, value string, width int) string {
	plain := fmt.Sprintf("%s: %s", label, value)
	plain = truncateRunes(plain, width)
	if model == nil || !model.useColor {
		return plain
	}

	prefix := label + ": "
	if !strings.HasPrefix(plain, prefix) {
		return initTeaBrightText(model, plain)
	}

	dim := initTeaDimStyle(model).Render(prefix)
	rest := strings.TrimPrefix(plain, prefix)
	if rest == "" {
		return dim
	}
	return dim + initTeaBrightText(model, rest)
}

func dimTimelineText(model *initTeaModel, value string) string {
	if !model.useColor {
		return value
	}
	return initTeaDimStyle(model).Render(value)
}

func initTeaStyle(model *initTeaModel) lipgloss.Style {
	if model != nil && model.renderer != nil {
		return model.renderer.NewStyle()
	}
	return lipgloss.NewStyle()
}

func initTeaAccentStyle(useColor bool) lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true)
	if useColor {
		return style.Foreground(lipgloss.Color("#22c55e"))
	}
	return style
}

func initTeaDimStyle(model *initTeaModel) lipgloss.Style {
	style := initTeaStyle(model)
	if model != nil && model.useColor {
		return style.Foreground(lipgloss.Color("#94a3b8"))
	}
	return style
}

func initTeaBrightText(model *initTeaModel, value string) string {
	style := initTeaStyle(model).Bold(true)
	if model != nil && model.useColor {
		style = style.Foreground(lipgloss.Color("#e2e8f0"))
	}
	return style.Render(value)
}

func renderSelectedOptionMarker(model *initTeaModel) string {
	if model == nil || !model.useColor {
		return "●"
	}
	return initTeaStyle(model).Bold(true).Foreground(lipgloss.Color("#60a5fa")).Render("●")
}

func actionTimelineText(model *initTeaModel, value string) string {
	if model == nil || !model.useColor {
		return value
	}
	return initTeaStyle(model).Bold(true).Foreground(lipgloss.Color("#60a5fa")).Render(value)
}

func initTeaBodyWidth(width int) int {
	return max(width-18, 16)
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit == 1 {
		return string(runes[:1])
	}
	return string(runes[:limit-1]) + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
