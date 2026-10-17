package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// ConfigBannerCmd lets users pick a banner theme interactively.
type ConfigBannerCmd struct{}

func (c *ConfigBannerCmd) Run(ctx context.Context, flags *RootFlags) error {
	writer := output.WriterFromContext(ctx)
	if err := bannerPickerUsable(writer, flags); err != nil {
		return err
	}

	cfg, err := config.Read()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	errOut := writer.Err
	if errOut == nil {
		errOut = os.Stderr
	}

	// The picker is chrome: it draws on stderr, so stderr decides color.
	model := newBannerPickerModel(BannerThemes(), cfg.BannerTheme, writer.ErrUseColor())
	model.ascii = output.ASCIIOnly()
	final, err := runBannerPicker(ctx, model,
		tea.WithOutput(errOut),
		tea.WithColorProfile(output.CharmProfile(writer.ErrColorProfile())),
	)
	if err != nil {
		return err
	}
	if !final.chosen {
		_, _ = fmt.Fprintln(errOut, "Cancelled.")
		if final.exitCode != 0 {
			// Ctrl-C or a signal: exit non-zero like the init wizard. The
			// notice above already told the user, so the error stays silent.
			return &ExitError{Code: final.exitCode, Err: &displayedError{err: errors.New("banner picker interrupted")}}
		}
		return nil
	}

	theme := final.themes[final.cursor]
	if err := cfg.Set("banner_theme", theme.Name); err != nil {
		return err
	}
	if err := config.Write(cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	_, _ = fmt.Fprintf(errOut, "Banner theme set to %s.\n", theme.Name)
	return nil
}

// bannerPickerUsable refuses to start the picker where nobody can answer it:
// --no-input, CI, stdin or stderr is not a terminal, or TERM=dumb (no cursor
// control for a redrawn frame).
func bannerPickerUsable(w *output.Writer, flags *RootFlags) error {
	reason := promptBlocker(flags)
	if reason == "" && output.DumbTerminal() {
		reason = "TERM=dumb"
	}
	if reason == "" && !w.ErrIsTTY() {
		reason = "stderr is not a terminal"
	}
	if reason == "" {
		return nil
	}
	return newHintedError(
		fmt.Errorf("interactive banner picker cannot run (%s); available themes: %s", reason, strings.Join(BannerThemeNames(), ", ")),
		errorUsageInvalid, ExitUsage,
		"set it directly with `nimbu config set --key banner_theme --value <name>`")
}

// runBannerPicker runs the picker program until the user selects or cancels.
// Bubble Tea owns raw mode and terminal restore on every exit path.
// SIGINT/SIGTERM are claimed from the root handler and forwarded to the
// program, so a signal cancels the picker once instead of also canceling ctx
// (killing the program) or force-exiting. bannerPickerFilter routes them
// through Update, so the frame is cleared and the exit code recorded.
func runBannerPicker(ctx context.Context, model bannerPickerModel, opts ...tea.ProgramOption) (bannerPickerModel, error) {
	opts = append([]tea.ProgramOption{tea.WithContext(ctx), tea.WithoutSignalHandler(), tea.WithFilter(bannerPickerFilter)}, opts...)
	program := tea.NewProgram(model, opts...)
	stopSignals := forwardProgramSignals(program)
	defer stopSignals()
	result, err := program.Run()
	final, ok := result.(bannerPickerModel)
	if !ok {
		final = model
	}
	if errors.Is(err, tea.ErrInterrupted) {
		if final.exitCode == 0 {
			final.exitCode = ExitInterrupted
		}
		final.chosen = false
		return final, nil
	}
	if err != nil {
		return model, fmt.Errorf("banner picker: %w", err)
	}
	return final, nil
}

// bannerStopMsg ends the picker for a signal, with the exit code to report.
type bannerStopMsg struct{ code int }

// Exit codes for a picker stopped by Ctrl-C or a signal (128 + signal).
const (
	bannerExitInterrupted = ExitInterrupted // ctrl+c / SIGINT
	bannerExitTerminated  = 143             // SIGTERM
)

// bannerPickerFilter turns a forwarded SIGINT (InterruptMsg) or SIGTERM
// (QuitMsg) into a bannerStopMsg. Bubble Tea acts on those messages before
// Update would see them, which left the full picker frame in the scrollback
// and exited 0. The picker's own tea.Quit (done already set) passes through.
func bannerPickerFilter(model tea.Model, msg tea.Msg) tea.Msg {
	m, _ := model.(bannerPickerModel)
	if m.done {
		return msg
	}
	switch msg.(type) {
	case tea.InterruptMsg:
		return bannerStopMsg{code: bannerExitInterrupted}
	case tea.QuitMsg:
		return bannerStopMsg{code: bannerExitTerminated}
	}
	return msg
}

const (
	bannerPickerDefaultWidth  = 80
	bannerPickerDefaultHeight = 24
	bannerPickerMaxVisible    = 7
	// bannerPickerChrome counts the title, the blank line under it, the blank
	// line above the hint and the hint itself.
	bannerPickerChrome = 4
)

// bannerPickerModel is an inline Bubble Tea picker: a windowed list of theme
// names with a selection marker, plus a color preview when color is on.
type bannerPickerModel struct {
	themes []BannerTheme
	banner []string
	cursor int
	width  int
	height int
	color  bool // render color previews
	ascii  bool // NIMBU_ASCII, TERM=dumb or a non-UTF-8 locale: no Unicode glyphs
	dark   bool // terminal background is dark (assumed until the terminal says otherwise)
	done   bool
	chosen bool
	// exitCode is non-zero when ctrl+c or a signal ended the picker.
	exitCode int
}

func newBannerPickerModel(themes []BannerTheme, current string, color bool) bannerPickerModel {
	m := bannerPickerModel{
		themes: themes,
		banner: strings.Split(strings.Trim(serverBanner, "\n"), "\n"),
		color:  color,
		dark:   true,
	}
	for i, t := range themes {
		if t.Name == current {
			m.cursor = i
			break
		}
	}
	return m
}

func (m bannerPickerModel) Init() tea.Cmd {
	if !m.color {
		return nil
	}
	// Render the dark palette first and treat the reply as an upgrade: some
	// terminals never answer the background query.
	return tea.RequestBackgroundColor
}

func (m bannerPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
	case tea.ResumeMsg:
		// Back from Ctrl-Z: the window may have been resized meanwhile.
		return m, tea.RequestWindowSize
	case bannerStopMsg:
		m.done, m.exitCode = true, msg.code
		return m, tea.Quit
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.done, m.exitCode = true, bannerExitInterrupted
			return m, tea.Quit
		case "ctrl+d", "esc", "q":
			m.done = true
			return m, tea.Quit
		case "ctrl+z":
			// Raw mode delivers Ctrl-Z as a key; suspend through Bubble Tea so
			// the terminal is restored (a no-op on Windows).
			return m, tea.Suspend
		case "enter":
			m.done, m.chosen = true, true
			return m, tea.Quit
		case "up", "k", "shift+tab":
			m.cursor = max(m.cursor-1, 0)
		case "down", "j", "tab":
			m.cursor = min(m.cursor+1, len(m.themes)-1)
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.themes) - 1
		}
	}
	return m, nil
}

func (m bannerPickerModel) View() tea.View {
	if m.done {
		// An empty final frame clears the picker before the result is printed.
		return tea.NewView("")
	}
	return tea.NewView(m.render())
}

func (m bannerPickerModel) size() (int, int) {
	w, h := m.width, m.height
	if w <= 0 {
		w = bannerPickerDefaultWidth
	}
	if h <= 0 {
		h = bannerPickerDefaultHeight
	}
	return w, h
}

// render lays the picker out for the current terminal size. Every line is cut
// to the terminal width, so the frame height is exactly its line count and
// redraws never stack on narrow terminals.
func (m bannerPickerModel) render() string {
	w, h := m.size()
	accent := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle()
	if m.color {
		palette := output.PaletteFor(m.dark)
		accent = accent.Foreground(lipgloss.Color(palette.Success))
		dim = dim.Foreground(lipgloss.Color(palette.Muted))
	}

	// Keep one row free so an inline frame never fills the whole screen.
	budget := h - 1 - bannerPickerChrome
	preview := m.preview(w, budget)
	listBudget := budget
	if len(preview) > 0 {
		listBudget -= len(preview) + 1
	}
	visible := max(min(listBudget, min(bannerPickerMaxVisible, len(m.themes))), 1)

	rows := make([]string, 0, h)
	rows = append(rows,
		lipgloss.NewStyle().Bold(true).Render("Choose a banner theme")+"  "+dim.Render(fmt.Sprintf("%d/%d", m.cursor+1, len(m.themes))),
		"")
	if len(preview) > 0 {
		rows = append(rows, preview...)
		rows = append(rows, "")
	}
	start := min(max(m.cursor-visible/2, 0), len(m.themes)-visible)
	for i := start; i < start+visible; i++ {
		if i == m.cursor {
			rows = append(rows, accent.Render("> "+m.themes[i].Label))
			continue
		}
		rows = append(rows, "  "+m.themes[i].Label)
	}
	hint := "↑/↓ move · enter select · esc cancel"
	if m.ascii {
		hint = "up/down move - enter select - esc cancel"
	}
	rows = append(rows, "", dim.Render(hint))

	for i, row := range rows {
		rows[i] = ansi.Truncate(row, w, "")
	}
	return strings.Join(rows, "\n")
}

// preview returns the color preview for the selected theme: the full banner
// when it fits, a row of swatches when only that fits, and nothing when color
// is off (every theme would look the same) or space is too tight. ASCII-only
// output skips the block-art banner and draws the swatches with "#".
func (m bannerPickerModel) preview(width, budget int) []string {
	if !m.color {
		return nil
	}
	palette := m.themes[m.cursor].PaletteFor(m.dark)
	colorAt := func(i int) lipgloss.Style {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette[i%len(palette)]))
	}
	minList := min(3, len(m.themes))

	bannerWidth := 0
	for _, line := range m.banner {
		bannerWidth = max(bannerWidth, ansi.StringWidth(line))
	}
	if !m.ascii && width >= bannerWidth+2 && budget-(len(m.banner)+1) >= minList {
		lines := make([]string, len(m.banner))
		for i, line := range m.banner {
			lines[i] = "  " + colorAt(i).Render(line)
		}
		return lines
	}
	if budget-2 >= minList {
		var swatches strings.Builder
		swatches.WriteString("  ")
		swatch := "██"
		if m.ascii {
			swatch = "##"
		}
		for i := range palette {
			swatches.WriteString(colorAt(i).Render(swatch))
		}
		return []string{swatches.String()}
	}
	return nil
}
