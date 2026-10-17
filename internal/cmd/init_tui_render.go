package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Layout limits. Below the minimum the wizard shows a resize notice instead of
// a clipped frame; the big banner only shows when there is room to spare.
const (
	initMinWidth        = 40
	initMinHeight       = 12
	initBannerMinHeight = 30
	initMinListRows     = 3
)

// initLayout is one way to lay the frame out; renderInitTea tries them from
// roomiest to tightest and keeps the first that fits the terminal.
type initLayout struct {
	bigHeader bool // framed ASCII banner instead of a one-line title
	compact   bool // no rail lines between finished steps
	hidden    int  // earliest finished steps folded into one summary line
	listRows  int  // option rows available to the active list
}

func (m *initTeaModel) tooSmall() bool {
	return m.width < initMinWidth || m.height < initMinHeight
}

func renderInitTea(m *initTeaModel) string {
	if m.tooSmall() {
		if m.quitting || m.phase == initTeaPhaseDone {
			// The closing frame stays in the scrollback: never let the resize
			// notice be it, or the next-step command is lost.
			return renderInitClosingSmall(m)
		}
		return clampInitLines(m, renderInitTooSmall(m))
	}
	candidates := []initLayout{{compact: false}, {compact: true}}
	if m.showBanner && m.height >= initBannerMinHeight && m.width >= initBannerWidth() {
		candidates = append([]initLayout{{bigHeader: true}}, candidates...)
	}
	for _, layout := range candidates {
		if frame, ok := fitInitLayout(m, layout); ok {
			return clampInitLines(m, frame)
		}
	}
	for hidden := 1; hidden <= len(m.transcript); hidden++ {
		if frame, ok := fitInitLayout(m, initLayout{compact: true, hidden: hidden}); ok {
			return clampInitLines(m, frame)
		}
	}
	// Nothing fits even folded: show the tightest frame; the inline renderer
	// keeps its bottom rows, which hold the active prompt.
	layout := initLayout{compact: true, hidden: len(m.transcript), listRows: initMinListRows}
	return clampInitLines(m, renderInitFrame(m, layout))
}

// fitInitLayout measures the frame without option rows, then gives the list
// whatever height is left. It fails when the chrome alone, or the chrome plus
// a minimal list, is taller than the terminal.
func fitInitLayout(m *initTeaModel, layout initLayout) (string, bool) {
	layout.listRows = 0
	chrome := renderInitFrame(m, layout)
	wanted := m.listRowsWanted()
	if wanted == 0 {
		return chrome, lipgloss.Height(chrome) <= m.height
	}
	avail := m.height - lipgloss.Height(chrome)
	if avail < min(wanted, initMinListRows) {
		return "", false
	}
	layout.listRows = min(wanted, avail)
	return renderInitFrame(m, layout), true
}

// listRowsWanted is how many option rows the active step could show.
func (m *initTeaModel) listRowsWanted() int {
	if m.phase != initTeaPhasePrompt {
		return 0
	}
	switch m.step {
	case initTeaStepDirectory, initTeaStepConfirm:
		return 0
	}
	return len(m.filteredOptions())
}

func renderInitFrame(m *initTeaModel, layout initLayout) string {
	lines := []string{renderInitHeader(m, layout.bigHeader)}
	if layout.bigHeader {
		lines = append(lines, "")
	}
	lines = append(lines, m.theme.muted(m.theme.glyphs.Top)+" "+m.theme.value("Let's set up a new project."))
	lines = append(lines, renderInitTimeline(m, layout)...)
	return strings.Join(lines, "\n")
}

// renderInitTooSmall replaces the wizard below the minimum size, wrapped so the
// whole notice stays readable however narrow the terminal is.
func renderInitTooSmall(m *initTeaModel) string {
	width := max(m.width, 1)
	var lines []string
	add := func(style func(string) string, text string) {
		for _, line := range strings.Split(ansi.Wrap(text, width, " "), "\n") {
			lines = append(lines, style(strings.TrimRight(line, " ")))
		}
	}
	add(m.theme.value, "Nimbu init")
	times := m.theme.glyphs.Times
	add(func(s string) string { return s }, fmt.Sprintf("Terminal too small: %d%s%d, needs %d%s%d.", m.width, times, m.height, initMinWidth, times, initMinHeight))
	if m.phase == initTeaPhaseApply {
		// esc is held while files are written; do not offer it.
		add(m.theme.muted, "Writing files"+m.theme.glyphs.Ellipsis+" enlarge the window to follow along.")
	} else {
		add(m.theme.muted, "Enlarge the window, or press esc to cancel.")
	}
	return strings.Join(lines, "\n")
}

// renderInitClosingSmall is the last frame below the minimum size: only the
// closing line ("Done!" with the next-step command, or "Cancelled."), wrapped
// to the width, plus the blank line Bubble Tea clears on exit.
func renderInitClosingSmall(m *initTeaModel) string {
	width := max(m.width, 1)
	var lines []string
	switch {
	case m.phase == initTeaPhaseDone:
		lines = renderInitTeaDoneFooter(m, width)
	case m.err == nil || errors.Is(m.err, errInitCancelled):
		lines = wrapInitLine(m.theme.muted(m.theme.glyphs.Bottom+" Cancelled."), width)
	}
	return strings.Join(append(lines, ""), "\n")
}

func initBannerLines() []string {
	return strings.Split(strings.Trim(serverBanner, "\n"), "\n")
}

func initBannerWidth() int {
	widest := 0
	for _, line := range initBannerLines() {
		widest = max(widest, lipgloss.Width(line))
	}
	return widest + 6
}

func renderInitHeader(m *initTeaModel, big bool) string {
	if !big {
		return m.theme.done("Nimbu init")
	}
	lines := initBannerLines()
	widest := initBannerWidth() - 6
	border := m.theme.style(m.theme.Border, false)
	palette := m.banner.PaletteFor(m.isDark)

	frameWidth := widest + 4
	rows := []string{
		border.Render(framedBannerEdge(frameWidth-2, " nimbu ")),
		border.Render("|" + strings.Repeat(" ", frameWidth) + "|"),
	}
	for idx, line := range lines {
		padded := line + strings.Repeat(" ", widest-lipgloss.Width(line))
		text := m.theme.style(m.theme.bannerLineColor(palette, idx), false).Render(padded)
		rows = append(rows, border.Render("|  ")+text+border.Render("  |"))
	}
	rows = append(rows, border.Render("+"+strings.Repeat("-", frameWidth)+"+"))
	return strings.Join(rows, "\n")
}

type initTimelineState string

const (
	initTimelineDone   initTimelineState = "done"
	initTimelineActive initTimelineState = "active"
)

func renderInitTimeline(m *initTeaModel, layout initLayout) []string {
	contentWidth := max(m.width-2, 1)
	rail := renderTimelineConnector(m)
	lines := make([]string, 0, len(m.transcript)*2+12)
	add := func(entry []string) {
		if !layout.compact || len(lines) == 0 {
			lines = append(lines, rail)
		}
		lines = append(lines, entry...)
	}

	hidden := min(layout.hidden, len(m.transcript))
	if hidden > 0 {
		noun := "steps"
		if hidden == 1 {
			noun = "step"
		}
		add(renderTimelineEntry(m, initTimelineDone, []string{m.theme.muted(fmt.Sprintf("%d earlier %s", hidden, noun))}))
	}
	for _, entry := range m.transcript[hidden:] {
		add(renderTimelineEntry(m, initTimelineDone, transcriptEntryLines(m, entry, contentWidth)))
	}

	switch {
	case m.phase == initTeaPhaseDone:
		lines = append(lines, rail)
		lines = append(lines, renderInitTeaDoneFooter(m, m.width)...)
	case m.quitting:
		// The last frame stays in the scrollback: replace the live prompt with
		// a closing line. Errors are printed by the command after exit.
		if m.err == nil || errors.Is(m.err, errInitCancelled) {
			lines = append(lines, rail, m.theme.muted(m.theme.glyphs.Bottom+" Cancelled."))
		}
	default:
		if active := activeTimelineLines(m, contentWidth, layout.listRows); len(active) > 0 {
			lines = append(lines, rail)
			lines = append(lines, renderTimelineEntry(m, initTimelineActive, active)...)
		}
	}
	if m.quitting {
		// Bubble Tea clears the frame's last line when it exits; give it a
		// blank one so the closing line survives.
		lines = append(lines, "")
	}
	return lines
}

func renderTimelineEntry(m *initTeaModel, state initTimelineState, lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	marker := m.theme.done(m.theme.glyphs.Done)
	if state == initTimelineActive {
		marker = m.theme.active(m.theme.glyphs.Active)
		if m.phase == initTeaPhaseLoading || m.phase == initTeaPhaseApply {
			marker = m.spinner.View()
		}
	}
	rows := make([]string, 0, len(lines))
	for idx, line := range lines {
		lead := marker
		if idx > 0 {
			lead = renderTimelineConnector(m)
		}
		if line == "" {
			rows = append(rows, lead)
			continue
		}
		rows = append(rows, lead+" "+line)
	}
	return rows
}

func renderTimelineConnector(m *initTeaModel) string {
	return m.theme.muted(m.theme.glyphs.Rail)
}

func transcriptEntryLines(m *initTeaModel, entry initTranscriptEntry, width int) []string {
	if text := strings.TrimSpace(entry.Text); text != "" {
		return []string{m.theme.truncate(text, width)}
	}
	if strings.TrimSpace(entry.Label) == "" {
		return []string{m.theme.truncate(entry.Value, width)}
	}
	return []string{renderLabelValueLine(m, entry.Label, entry.Value, width)}
}

// renderInitTeaDoneFooter is the closing line that stays in the scrollback. It
// is never truncated: when the next-step command does not fit beside the
// label it moves to its own line(s), so it can always be read and copied.
func renderInitTeaDoneFooter(m *initTeaModel, width int) []string {
	command := initNextStepCommand(m.result.Path)
	lead := m.theme.muted(m.theme.glyphs.Bottom) + " " + m.theme.done("Done!")
	if command == "" {
		// In-place into the current directory — nothing to cd into.
		return wrapInitLine(lead+" "+m.theme.muted("Your project is ready."), width)
	}
	label := lead + " " + m.theme.muted("To start working, run:")
	if line := label + " " + m.theme.value(command); width <= 0 || lipgloss.Width(line) <= width {
		return []string{line}
	}
	lines := wrapInitLine(label, width)
	for _, part := range strings.Split(ansi.Hardwrap(command, max(width-2, 1), false), "\n") {
		lines = append(lines, "  "+m.theme.value(part))
	}
	return lines
}

// wrapInitLine word-wraps a styled line to width cells.
func wrapInitLine(line string, width int) []string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return []string{line}
	}
	return strings.Split(ansi.Wrap(line, width, ""), "\n")
}

// renderLabelValueLine renders "Label: value" with a muted label and a bright
// value, truncated by display width. The value is plain text.
func renderLabelValueLine(m *initTeaModel, label, value string, width int) string {
	return renderLabelLine(m, label, m.theme.value(m.theme.truncate(value, max(width-lipgloss.Width(label)-2, 1))), width)
}

// renderLabelPathLine is renderLabelValueLine for a filesystem path: the home
// directory shows as ~ and a long path loses its start, never its end, so the
// directory about to be written stays visible.
func renderLabelPathLine(m *initTeaModel, label, path string, width int) string {
	return renderLabelLine(m, label, m.theme.value(truncateInitPath(m, path, max(width-lipgloss.Width(label)-2, 1))), width)
}

// initHomeDir is swapped in tests.
var initHomeDir = os.UserHomeDir

// truncateInitPath shortens path to width cells, keeping its tail.
func truncateInitPath(m *initTeaModel, path string, width int) string {
	if width <= 0 {
		return ""
	}
	if home, err := initHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			path = filepath.Join("~", rel)
		}
	}
	cells := lipgloss.Width(path)
	if cells <= width {
		return path
	}
	tail := m.theme.glyphs.Ellipsis
	if lipgloss.Width(tail) >= width {
		return m.theme.truncate(path, width)
	}
	return ansi.TruncateLeft(path, cells-width+lipgloss.Width(tail), tail)
}

// renderLabelLine is renderLabelValueLine for an already-styled value (such as
// a text input), which is truncated ANSI-aware instead of restyled.
func renderLabelLine(m *initTeaModel, label, styled string, width int) string {
	prefix := label + ": "
	if lipgloss.Width(prefix) >= width {
		return m.theme.muted(m.theme.truncate(prefix, width))
	}
	return m.theme.muted(prefix) + m.theme.truncate(styled, width-lipgloss.Width(prefix))
}

// clampInitLines guarantees no frame line is wider than the terminal.
func clampInitLines(m *initTeaModel, frame string) string {
	width := m.width
	if width <= 0 {
		return frame
	}
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		if lipgloss.Width(line) > width {
			lines[i] = m.theme.truncate(line, width)
		}
	}
	return strings.Join(lines, "\n")
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
