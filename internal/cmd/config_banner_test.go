package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/nimbu/cli/internal/output"
)

func pickerKey(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

func pickerUpdate(t *testing.T, m bannerPickerModel, msg tea.Msg) (bannerPickerModel, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	got, ok := next.(bannerPickerModel)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	return got, cmd
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestBannerPickerStartsOnConfiguredTheme(t *testing.T) {
	m := newBannerPickerModel(BannerThemes(), "ocean", false)
	if got := m.themes[m.cursor].Name; got != "ocean" {
		t.Fatalf("cursor on %q, want ocean", got)
	}
	if m := newBannerPickerModel(BannerThemes(), "unknown", false); m.cursor != 0 {
		t.Fatalf("unknown theme cursor = %d, want 0", m.cursor)
	}
}

func TestBannerPickerMovement(t *testing.T) {
	last := len(BannerThemes()) - 1
	tests := []struct {
		name  string
		start int
		key   tea.KeyPressMsg
		want  int
	}{
		{"down arrow", 0, tea.KeyPressMsg{Code: tea.KeyDown}, 1},
		{"j", 0, pickerKey('j', "j"), 1},
		{"tab", 0, tea.KeyPressMsg{Code: tea.KeyTab}, 1},
		{"up arrow", 2, tea.KeyPressMsg{Code: tea.KeyUp}, 1},
		{"k", 2, pickerKey('k', "k"), 1},
		{"up clamps at top", 0, tea.KeyPressMsg{Code: tea.KeyUp}, 0},
		{"down clamps at bottom", last, tea.KeyPressMsg{Code: tea.KeyDown}, last},
		{"home", 5, tea.KeyPressMsg{Code: tea.KeyHome}, 0},
		{"g", 5, pickerKey('g', "g"), 0},
		{"end", 0, tea.KeyPressMsg{Code: tea.KeyEnd}, last},
		{"G", 0, pickerKey('G', "G"), last},
		{"unbound key is ignored", 3, pickerKey('x', "x"), 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBannerPickerModel(BannerThemes(), "", false)
			m.cursor = tt.start
			got, cmd := pickerUpdate(t, m, tt.key)
			if got.cursor != tt.want {
				t.Fatalf("cursor = %d, want %d", got.cursor, tt.want)
			}
			if cmd != nil || got.done {
				t.Fatalf("movement must not quit (done=%v)", got.done)
			}
		})
	}
}

// Regression: ctrl+c did nothing because raw mode disabled ISIG and the key
// loop had no case for it.
func TestBannerPickerCancelKeys(t *testing.T) {
	keys := map[string]tea.KeyPressMsg{
		"ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
		"ctrl+d": {Code: 'd', Mod: tea.ModCtrl},
		"esc":    {Code: tea.KeyEscape},
		"q":      pickerKey('q', "q"),
	}
	for name, key := range keys {
		t.Run(name, func(t *testing.T) {
			m, cmd := pickerUpdate(t, newBannerPickerModel(BannerThemes(), "", true), key)
			if !m.done || m.chosen {
				t.Fatalf("done=%v chosen=%v, want cancelled", m.done, m.chosen)
			}
			if !isQuit(cmd) {
				t.Fatal("cancel must quit the program")
			}
			if v := m.View(); v.Content != "" {
				t.Fatalf("final frame = %q, want it cleared", v.Content)
			}
			// ctrl+c is an interrupt (exit 130); the others cancel cleanly.
			want := 0
			if name == "ctrl+c" {
				want = ExitInterrupted
			}
			if m.exitCode != want {
				t.Fatalf("exitCode = %d, want %d", m.exitCode, want)
			}
		})
	}
}

// Regression: a forwarded SIGINT/SIGTERM bypassed Update, so the full picker
// frame stayed in the scrollback and the command exited 0.
func TestBannerPickerFilterRoutesSignalsThroughUpdate(t *testing.T) {
	live := newBannerPickerModel(BannerThemes(), "", false)
	if got := bannerPickerFilter(live, tea.InterruptMsg{}); got != (bannerStopMsg{code: 130}) {
		t.Fatalf("SIGINT -> %#v, want stop 130", got)
	}
	if got := bannerPickerFilter(live, tea.QuitMsg{}); got != (bannerStopMsg{code: 143}) {
		t.Fatalf("SIGTERM -> %#v, want stop 143", got)
	}
	if got := bannerPickerFilter(live, tea.KeyPressMsg{Code: tea.KeyDown}); got != (tea.KeyPressMsg{Code: tea.KeyDown}) {
		t.Fatalf("other messages must pass through, got %#v", got)
	}
	done := live
	done.done = true
	if _, ok := bannerPickerFilter(done, tea.QuitMsg{}).(tea.QuitMsg); !ok {
		t.Fatal("the picker's own quit must pass through")
	}

	m, cmd := pickerUpdate(t, live, bannerStopMsg{code: 143})
	if !m.done || m.chosen || m.exitCode != 143 || !isQuit(cmd) {
		t.Fatalf("stop: done=%v chosen=%v exit=%d quit=%v", m.done, m.chosen, m.exitCode, isQuit(cmd))
	}
	if v := m.View(); v.Content != "" {
		t.Fatalf("final frame = %q, want it cleared", v.Content)
	}
}

func TestBannerPickerSignalsThroughProgram(t *testing.T) {
	for name, tc := range map[string]struct {
		msg  tea.Msg
		want int
	}{
		"SIGINT":  {tea.InterruptMsg{}, 130},
		"SIGTERM": {tea.QuitMsg{}, 143},
	} {
		t.Run(name, func(t *testing.T) {
			in, inW := io.Pipe()
			defer func() { _ = inW.Close() }()
			program := tea.NewProgram(newBannerPickerModel(BannerThemes(), "", false),
				tea.WithInput(in),
				tea.WithOutput(io.Discard),
				tea.WithWindowSize(80, 24),
				tea.WithoutSignals(),
				tea.WithFilter(bannerPickerFilter),
			)
			go program.Send(tc.msg)
			result, err := program.Run()
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			final := result.(bannerPickerModel)
			if !final.done || final.chosen || final.exitCode != tc.want {
				t.Fatalf("done=%v chosen=%v exit=%d, want exit %d", final.done, final.chosen, final.exitCode, tc.want)
			}
		})
	}
}

// Raw mode delivers Ctrl-Z as a key; the picker must ask Bubble Tea to
// suspend instead of dropping it.
func TestBannerPickerCtrlZSuspends(t *testing.T) {
	m, cmd := pickerUpdate(t, newBannerPickerModel(BannerThemes(), "", false), tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+z must return a command")
	}
	if _, ok := cmd().(tea.SuspendMsg); !ok || m.done {
		t.Fatalf("ctrl+z: done=%v, want a suspend", m.done)
	}
	if _, cmd := pickerUpdate(t, m, tea.ResumeMsg{}); cmd == nil {
		t.Fatal("resume must re-read the window size")
	}
}

func TestBannerPickerASCIIFrame(t *testing.T) {
	m := newBannerPickerModel(BannerThemes(), "", true)
	m.ascii = true
	frame := ansi.Strip(m.render())
	for _, r := range frame {
		if r > 0x7f {
			t.Fatalf("ASCII frame has %q:\n%s", r, frame)
		}
	}
	if !strings.Contains(frame, "up/down move") || !strings.Contains(frame, "##") {
		t.Fatalf("ASCII frame lacks hint or swatches:\n%s", frame)
	}
}

func TestBannerPickerEnterSelects(t *testing.T) {
	m := newBannerPickerModel(BannerThemes(), "neon", false)
	m, cmd := pickerUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.done || !m.chosen || m.themes[m.cursor].Name != "neon" {
		t.Fatalf("done=%v chosen=%v theme=%q", m.done, m.chosen, m.themes[m.cursor].Name)
	}
	if !isQuit(cmd) {
		t.Fatal("enter must quit the program")
	}
}

func TestBannerPickerAsksForBackgroundOnlyWithColor(t *testing.T) {
	if cmd := newBannerPickerModel(BannerThemes(), "", false).Init(); cmd != nil {
		t.Fatal("no color: the picker must not query the terminal background")
	}
	cmd := newBannerPickerModel(BannerThemes(), "", true).Init()
	if cmd == nil {
		t.Fatal("color: the picker should request the background color")
	}
}

func TestBannerPickerUsesLightPaletteOnLightBackground(t *testing.T) {
	m := newBannerPickerModel(BannerThemes(), "sakura", true)
	m, _ = pickerUpdate(t, m, tea.BackgroundColorMsg{Color: bannerHex("#ffffff")})
	if m.dark {
		t.Fatal("white background reported as dark")
	}
	theme, _ := BannerThemeByName("sakura")
	frame := m.render()
	if !strings.Contains(frame, truecolorFG(theme.Light[0])) {
		t.Fatalf("frame does not use the light palette %v:\n%q", theme.Light, frame)
	}
	if strings.Contains(frame, truecolorFG(theme.Palette[0])) {
		t.Fatal("frame still uses the dark palette on a light background")
	}
}

func truecolorFG(hex string) string {
	r, g, b := bannerHex(hex).RGB255()
	return fmt.Sprintf("38;2;%d;%d;%dm", r, g, b)
}

func TestBannerPickerFrameNoColor80x24(t *testing.T) {
	m := newBannerPickerModel(BannerThemes(), "gradient", false)
	m, _ = pickerUpdate(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	want := strings.Join([]string{
		"Choose a banner theme  3/18",
		"",
		"  Default (terminal text color)",
		"  Rainbow",
		"> Gradient",
		"  Ocean",
		"  Sunset",
		"  Forest",
		"  Neon",
		"",
		"↑/↓ move · enter select · esc cancel",
	}, "\n")
	if got := ansi.Strip(m.render()); got != want {
		t.Fatalf("frame mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBannerPickerFrameColor80x24(t *testing.T) {
	m := newBannerPickerModel(BannerThemes(), "ocean", true)
	m, _ = pickerUpdate(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	banner := strings.Split(strings.Trim(serverBanner, "\n"), "\n")
	if len(lines) > 23 {
		t.Fatalf("frame is %d lines, want it to fit 24 rows with one spare", len(lines))
	}
	for i, line := range banner {
		if lines[2+i] != "  "+line {
			t.Fatalf("banner row %d = %q", i, lines[2+i])
		}
	}
	if !strings.Contains(strings.Join(lines, "\n"), "> Ocean") {
		t.Fatal("selected theme not marked")
	}
	assertFrameFits(t, lines, 80)
}

// Regression: on terminals narrower than the banner the old redraw assumed a
// fixed height, so wrapped lines stacked frames on every keypress.
func TestBannerPickerFrameColor40Cols(t *testing.T) {
	m := newBannerPickerModel(BannerThemes(), "rainbow", true)
	m, _ = pickerUpdate(t, m, tea.WindowSizeMsg{Width: 40, Height: 24})
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	want := strings.Join([]string{
		"Choose a banner theme  2/18",
		"",
		"  ████████████",
		"",
		"  Default (terminal text color)",
		"> Rainbow",
		"  Gradient",
		"  Ocean",
		"  Sunset",
		"  Forest",
		"  Neon",
		"",
		"↑/↓ move · enter select · esc cancel",
	}, "\n")
	if got := strings.Join(lines, "\n"); got != want {
		t.Fatalf("frame mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
	assertFrameFits(t, lines, 40)
}

func TestBannerPickerFrameTinyTerminal(t *testing.T) {
	m := newBannerPickerModel(BannerThemes(), "sakura", true)
	m, _ = pickerUpdate(t, m, tea.WindowSizeMsg{Width: 20, Height: 7})
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	if len(lines) > 6 {
		t.Fatalf("frame is %d lines on a 7-row terminal:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if !strings.Contains(strings.Join(lines, "\n"), "> Sakura") {
		t.Fatalf("selected theme must stay visible:\n%s", strings.Join(lines, "\n"))
	}
	assertFrameFits(t, lines, 20)
}

func assertFrameFits(t *testing.T, lines []string, width int) {
	t.Helper()
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > width {
			t.Fatalf("line %d is %d cells wide, terminal is %d: %q", i, w, width, line)
		}
	}
}

func TestBannerPickerListKeepsCursorVisible(t *testing.T) {
	m := newBannerPickerModel(BannerThemes(), "sakura", false)
	frame := ansi.Strip(m.render())
	if !strings.Contains(frame, "> Sakura") || !strings.Contains(frame, "18/18") {
		t.Fatalf("last theme not visible:\n%s", frame)
	}
}

func TestBannerPickerUsable(t *testing.T) {
	w := &output.Writer{Err: &bytes.Buffer{}}
	stubInteractivePrompt(t, true)
	err := bannerPickerUsable(w, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "stderr is not a terminal") || !strings.Contains(err.Error(), "rainbow") {
		t.Fatalf("stderr not a terminal: err = %v", err)
	}
	desc := classifyError(err)
	if desc.Code != errorUsageInvalid || desc.ExitCode != ExitUsage {
		t.Fatalf("refusal = %s/%d, want %s/%d", desc.Code, desc.ExitCode, errorUsageInvalid, ExitUsage)
	}
	// The hinted command must be one `config set` actually parses.
	if !strings.Contains(desc.Hint, "nimbu config set --key banner_theme --value <name>") {
		t.Fatalf("hint = %q", desc.Hint)
	}
	parser, _, perr := newParser()
	if perr != nil {
		t.Fatal(perr)
	}
	if _, perr := parser.Parse([]string{"config", "set", "--key", "banner_theme", "--value", "ocean"}); perr != nil {
		t.Fatalf("hinted command does not parse: %v", perr)
	}

	for name, setup := range map[string]func(t *testing.T) *RootFlags{
		"no-input": func(t *testing.T) *RootFlags { stubInteractivePrompt(t, true); return &RootFlags{NoInput: true} },
		"CI": func(t *testing.T) *RootFlags {
			stubInteractivePrompt(t, true)
			t.Setenv("CI", "true")
			return &RootFlags{}
		},
		"stdin": func(t *testing.T) *RootFlags { stubInteractivePrompt(t, false); return &RootFlags{} },
		"TERM=dumb": func(t *testing.T) *RootFlags {
			stubInteractivePrompt(t, true)
			t.Setenv("TERM", "dumb")
			return &RootFlags{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := bannerPickerUsable(w, setup(t))
			if err == nil {
				t.Fatal("picker must refuse")
			}
			if desc := classifyError(err); desc.ExitCode != ExitUsage {
				t.Fatalf("exit = %d, want %d", desc.ExitCode, ExitUsage)
			}
		})
	}
}

// TERM=dumb has no cursor control for the redrawn frame: refuse before
// starting Bubble Tea, even on a real terminal.
func TestBannerPickerRefusesDumbTerminal(t *testing.T) {
	stubInteractivePrompt(t, true)
	t.Setenv("TERM", "dumb")
	err := bannerPickerUsable(&output.Writer{Err: &bytes.Buffer{}}, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "TERM=dumb") {
		t.Fatalf("err = %v, want a TERM=dumb refusal", err)
	}
}

func runPickerWithInput(t *testing.T, input string) (bannerPickerModel, string) {
	t.Helper()
	var out bytes.Buffer
	final, err := runBannerPicker(context.Background(), newBannerPickerModel(BannerThemes(), "", false),
		tea.WithInput(strings.NewReader(input)),
		tea.WithOutput(&out),
		tea.WithWindowSize(80, 24),
		tea.WithColorProfile(colorprofile.ASCII),
		tea.WithoutSignals(),
	)
	if err != nil {
		t.Fatalf("run picker: %v", err)
	}
	return final, out.String()
}

func TestRunBannerPickerSelectsThroughProgram(t *testing.T) {
	final, _ := runPickerWithInput(t, "jj\r")
	if !final.chosen || final.themes[final.cursor].Name != "gradient" {
		t.Fatalf("chosen=%v theme=%q, want gradient", final.chosen, final.themes[final.cursor].Name)
	}
}

func TestRunBannerPickerCtrlCCancelsThroughProgram(t *testing.T) {
	final, _ := runPickerWithInput(t, "j\x03")
	// done is only set by a key handler, so this proves ctrl+c reached the
	// model rather than the program ending on EOF.
	if !final.done || final.chosen || final.exitCode != ExitInterrupted {
		t.Fatalf("done=%v chosen=%v exit=%d, want ctrl+c to cancel with 130", final.done, final.chosen, final.exitCode)
	}
}
