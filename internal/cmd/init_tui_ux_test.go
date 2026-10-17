package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Regression: Bubble Tea v2 delivers a bracketed paste as tea.PasteMsg, which
// the wizard dropped, so pasting a site name or directory did nothing.
func TestInitTeaPasteReachesFilterAndDirectory(t *testing.T) {
	model := initGoldenModel(false, "site", 80, 24)
	model.cursor = 20
	model.Update(tea.PasteMsg{Content: "site-13"})
	if got := model.filterInput.Value(); got != "site-13" {
		t.Fatalf("filter = %q, want the pasted text", got)
	}
	options := model.filteredOptions()
	if len(options) != 1 || model.cursor != 0 {
		t.Fatalf("filtered to %d options with cursor %d, want 1 and 0", len(options), model.cursor)
	}

	model = initGoldenModel(false, "directory", 80, 24)
	model.directoryInput.SetValue("")
	model.Update(tea.PasteMsg{Content: "my-pasted-dir"})
	if got := model.answers.DirectoryName; got != "my-pasted-dir" {
		t.Fatalf("directory = %q, want the pasted text", got)
	}
	if !strings.Contains(ansi.Strip(initView(model)), "my-pasted-dir") {
		t.Fatalf("pasted directory not drawn:\n%s", ansi.Strip(initView(model)))
	}
}

// Raw mode delivers Ctrl-Z as a key; it must suspend through Bubble Tea in
// every phase instead of being dropped.
func TestInitTeaCtrlZSuspends(t *testing.T) {
	ctrlZ := tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}
	for name, model := range map[string]*initTeaModel{
		"prompt": initGoldenModel(false, "site", 80, 24),
		"apply":  initApplyModel(t),
	} {
		t.Run(name, func(t *testing.T) {
			_, cmd := model.Update(ctrlZ)
			if cmd == nil {
				t.Fatal("ctrl+z must return a command")
			}
			if _, ok := cmd().(tea.SuspendMsg); !ok {
				t.Fatal("ctrl+z must suspend")
			}
			if model.quitting || model.cancelHeld {
				t.Fatal("ctrl+z must not cancel")
			}
			if _, cmd := model.Update(tea.ResumeMsg{}); cmd == nil {
				t.Fatal("resume must re-read the window size")
			}
		})
	}
}

// Regression: the search line draws no caret, yet left/right moved the
// insertion point, so "site1", 4x left, "x" became "sxite1".
func TestInitTeaFilterEditsStayAtTheEnd(t *testing.T) {
	model := initGoldenModel(false, "site", 80, 24)
	for _, r := range "site1" {
		model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	for range 4 {
		model.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := model.filterInput.Value(); got != "site1x" {
		t.Fatalf("filter = %q, want site1x", got)
	}
}

// Regression: a wizard that finished or was cancelled below the minimum size
// left the clipped resize notice as its last frame, losing "Done!" and the
// next-step command.
func TestInitTeaClosingFrameBelowMinimumSize(t *testing.T) {
	done := newInitTeaTestModel(initPromptModel{})
	done.width, done.height = 30, 10
	done.phase = initTeaPhaseDone
	done.quitting = true
	done.result = initResult{Path: filepath.Join(t.TempDir(), "theme-zenjoy")}
	frame := ansi.Strip(initView(done))
	if !strings.Contains(frame, "Done!") || !strings.Contains(frame, "theme-zenjoy") || strings.Contains(frame, "too small") {
		t.Fatalf("done frame at 30x10:\n%s", frame)
	}
	if !strings.HasSuffix(frame, "\n") {
		t.Fatalf("closing frame must end with a blank line Bubble Tea may clear: %q", frame)
	}

	cancelled := initGoldenModel(false, "site", 30, 10)
	cancelled.err = errInitCancelled
	cancelled.quitting = true
	frame = ansi.Strip(initView(cancelled))
	if !strings.Contains(frame, "Cancelled.") || strings.Contains(frame, "too small") || !strings.HasSuffix(frame, "\n") {
		t.Fatalf("cancelled frame at 30x10: %q", frame)
	}
	for _, line := range strings.Split(frame, "\n") {
		if w := ansi.StringWidth(line); w > 30 {
			t.Fatalf("line wider than 30 cells: %q", line)
		}
	}

	apply := initApplyModel(t)
	apply.width, apply.height = 30, 10
	frame = ansi.Strip(initView(apply))
	if strings.Contains(frame, "esc") || !strings.Contains(frame, "Writing files") {
		t.Fatalf("apply notice must not offer esc while it is held:\n%s", frame)
	}
}

// Regression: the confirm step cut a long destination path at the tail,
// hiding the directory about to be written.
func TestInitTeaConfirmPathKeepsDirectoryName(t *testing.T) {
	orig := initHomeDir
	initHomeDir = func() (string, error) { return "/Users/someone", nil }
	t.Cleanup(func() { initHomeDir = orig })

	long := "/private/tmp/claude-501/-Users-peter-Development-Zenjoy-Internal-nimbu-go-cli/workspace/projects"
	for _, width := range []int{initMinWidth, 80} {
		model := initGoldenModel(false, "confirm", width, 24)
		model.outputDir = long
		model.answers.DirectoryName = "my-new-site"
		frame := ansi.Strip(initView(model))
		if !strings.Contains(frame, "my-new-site") {
			t.Fatalf("width %d: confirm frame hides the directory:\n%s", width, frame)
		}
	}

	model := initGoldenModel(false, "confirm", 80, 24)
	model.outputDir = "/Users/someone/code"
	model.answers.DirectoryName = "site"
	if frame := ansi.Strip(initView(model)); !strings.Contains(frame, "Path: "+filepath.Join("~", "code", "site")) {
		t.Fatalf("home should show as ~:\n%s", frame)
	}
}

// Regression: the hint line was cut from the right, so the esc segment was
// the first thing lost on narrow terminals.
func TestInitTeaHintAlwaysListsEsc(t *testing.T) {
	steps := map[string]initTeaStep{
		"repeatables": initTeaStepRepeatables,
		"bundles":     initTeaStepBundles,
		"site":        initTeaStepSite,
	}
	for name, step := range steps {
		for width := initMinWidth; width <= 80; width++ {
			for _, filter := range []string{"", "e"} {
				model := initGoldenModel(false, "site", width, 30)
				model.enterStep(step)
				if filter != "" && model.isFilterableStep() {
					model.filterInput.SetValue(filter)
				}
				want := "esc cancel"
				if model.filterActive() {
					want = "esc clear filter"
				}
				if frame := ansi.Strip(initView(model)); !strings.Contains(frame, want) {
					t.Fatalf("%s at %d cols (filter %q): no %q in\n%s", name, width, filter, want, frame)
				}
			}
		}
	}
}

// NIMBU_ASCII, TERM=dumb or a non-UTF-8 locale: every wizard glyph has an
// ASCII fallback.
func TestInitTeaASCIIFrames(t *testing.T) {
	for _, step := range []string{"site", "site-scrolled", "confirm", "loading", "directory"} {
		model := initGoldenModel(false, step, 60, 24)
		model.ascii = true
		model.spinner.Spinner = initSpinner(true)
		model.applyTheme()
		if step == "site" {
			model.filterInput.SetValue("site-1")
		}
		frames := []string{initView(model)}
		model.width = 30
		frames = append(frames, initView(model))
		for _, frame := range frames {
			for _, r := range ansi.Strip(frame) {
				if r > 0x7f {
					t.Fatalf("%s: non-ASCII %q in\n%s", step, r, ansi.Strip(frame))
				}
			}
		}
	}

	multi := initGoldenModel(false, "site", 60, 24)
	multi.ascii = true
	multi.applyTheme()
	multi.enterStep(initTeaStepRepeatables)
	multi.repeatables["header"] = struct{}{}
	frame := ansi.Strip(initView(multi))
	if !strings.Contains(frame, "[x] Header") || !strings.Contains(frame, "[ ] Text") {
		t.Fatalf("ASCII multi-select markers missing:\n%s", frame)
	}
}

// Regression: the bubbles default caret is reverse video on ANSI 7, nearly
// invisible on a light background.
func TestInitTeaCaretUsesTerminalForeground(t *testing.T) {
	model := initGoldenModel(true, "directory", 60, 24)
	model.isDark = false
	model.applyTheme()
	if got := model.directoryInput.Styles().Cursor.Color; got != nil {
		t.Fatalf("caret color = %v, want the terminal foreground (nil)", got)
	}
	if got := model.filterInput.Styles().Cursor.Color; got != nil {
		t.Fatalf("filter caret color = %v, want nil", got)
	}
}
