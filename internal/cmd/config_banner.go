package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// ConfigBannerCmd lets users pick a banner theme interactively.
type ConfigBannerCmd struct{}

func (c *ConfigBannerCmd) Run(ctx context.Context) error {
	writer := output.WriterFromContext(ctx)
	if err := bannerPickerUsable(writer, term.IsTerminal(int(os.Stdin.Fd())), os.Getenv("CI")); err != nil {
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
	final, err := runBannerPicker(ctx, model,
		tea.WithOutput(errOut),
		tea.WithColorProfile(colorprofileFromTermenv(writer.ErrColorProfile())),
	)
	if err != nil {
		return err
	}
	if !final.chosen {
		_, _ = fmt.Fprintln(errOut, "Cancelled.")
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
// stdin or stderr is not a terminal, or the process runs in CI.
func bannerPickerUsable(w *output.Writer, stdinTTY bool, ci string) error {
	if stdinTTY && shouldShowBanner(w.ErrIsTTY(), ci) {
		return nil
	}
	return fmt.Errorf("interactive banner picker needs a terminal; use 'nimbu config set banner_theme <name>' instead\navailable themes: %s",
		strings.Join(BannerThemeNames(), ", "))
}

// runBannerPicker runs the picker program until the user selects or cancels.
// Bubble Tea owns raw mode, SIGINT/SIGTERM and terminal restore on every exit
// path; an interrupt counts as a cancel.
func runBannerPicker(ctx context.Context, model bannerPickerModel, opts ...tea.ProgramOption) (bannerPickerModel, error) {
	opts = append([]tea.ProgramOption{tea.WithContext(ctx)}, opts...)
	result, err := tea.NewProgram(model, opts...).Run()
	if errors.Is(err, tea.ErrInterrupted) {
		return model, nil
	}
	if err != nil {
		return model, fmt.Errorf("banner picker: %w", err)
	}
	final, ok := result.(bannerPickerModel)
	if !ok {
		return model, nil
	}
	return final, nil
}

// colorprofileFromTermenv maps the CLI's per-stream color decision onto the
// profile Bubble Tea and Lipgloss v2 downsample to.
func colorprofileFromTermenv(p termenv.Profile) colorprofile.Profile {
	switch p {
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
	dark   bool // terminal background is dark (assumed until the terminal says otherwise)
	done   bool
	chosen bool
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
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+d", "esc", "q":
			m.done = true
			return m, tea.Quit
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
	ld := lipgloss.LightDark(m.dark)
	accent := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle()
	if m.color {
		accent = accent.Foreground(ld(lipgloss.Color("#15803d"), lipgloss.Color("#22c55e")))
		dim = dim.Foreground(ld(lipgloss.Color("#475569"), lipgloss.Color("#94a3b8")))
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
	rows = append(rows, "", dim.Render("↑/↓ move · enter select · esc cancel"))

	for i, row := range rows {
		rows[i] = ansi.Truncate(row, w, "")
	}
	return strings.Join(rows, "\n")
}

// preview returns the color preview for the selected theme: the full banner
// when it fits, a row of swatches when only that fits, and nothing when color
// is off (every theme would look the same) or space is too tight.
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
	if width >= bannerWidth+2 && budget-(len(m.banner)+1) >= minList {
		lines := make([]string, len(m.banner))
		for i, line := range m.banner {
			lines[i] = "  " + colorAt(i).Render(line)
		}
		return lines
	}
	if budget-2 >= minList {
		var swatches strings.Builder
		swatches.WriteString("  ")
		for i := range palette {
			swatches.WriteString(colorAt(i).Render("██"))
		}
		return []string{swatches.String()}
	}
	return nil
}
