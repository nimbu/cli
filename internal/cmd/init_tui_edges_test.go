package cmd

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// A ctrl+c held during the write exits 130 once the files are in place, the
// same as a SIGINT would; before, it silently ended in success.
func TestInitTeaHeldCtrlCDuringApplyExits130AfterWrite(t *testing.T) {
	model := initApplyModel(t)
	model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	model.Update(initTeaBootstrapDoneMsg{result: initResult{Path: "/tmp/theme-demo"}})

	err := initTeaOutcome(model, nil)
	if code := exitCode(t, err); code != initExitInterrupted {
		t.Fatalf("expected exit %d, got %d", initExitInterrupted, code)
	}
	if !strings.Contains(err.Error(), "/tmp/theme-demo") {
		t.Fatalf("expected error to say where the project was written, got %v", err)
	}
}

// A held esc only shows the finishing notice; the finished project is a success.
func TestInitTeaHeldEscDuringApplyStillSucceeds(t *testing.T) {
	model := initApplyModel(t)
	model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model.Update(initTeaBootstrapDoneMsg{result: initResult{Path: "/tmp/theme-demo"}})
	if err := initTeaOutcome(model, nil); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

// SIGTERM ends the program gracefully and Bubble Tea renders the model once
// more; that last frame must close the wizard, not leave a live prompt.
func TestInitTeaSigtermDuringPromptDrawsClosingFrame(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{Sites: []initSiteChoice{{Label: "Demo"}}})
	model.enterStep(initTeaStepSite)
	if got := initTeaFilter(model, tea.QuitMsg{}); got != (tea.QuitMsg{}) {
		t.Fatalf("expected quit to pass through, got %#v", got)
	}
	view := initView(model)
	if !strings.Contains(view, "Cancelled.") || strings.Contains(view, "enter select") {
		t.Fatalf("expected a closing frame without the live prompt, got:\n%s", view)
	}
}

func TestInitSignalMsgMatchesBubbleTea(t *testing.T) {
	if _, ok := programSignalMsg(os.Interrupt).(tea.InterruptMsg); !ok {
		t.Fatal("expected SIGINT to become an InterruptMsg")
	}
	if _, ok := programSignalMsg(syscall.SIGTERM).(tea.QuitMsg); !ok {
		t.Fatal("expected SIGTERM to become a QuitMsg")
	}
}

// The "Done!" line is the one thing left in the scrollback: on a narrow
// terminal the next-step command wraps onto its own line instead of being cut
// off with an ellipsis.
func TestInitTeaDoneFooterWrapsInsteadOfTruncating(t *testing.T) {
	dir := t.TempDir()
	project := dir + "/theme-customer-site-with-a-long-name"
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project+"/package.json", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project+"/pnpm-lock.yaml", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	command := "cd theme-customer-site-with-a-long-name && pnpm install"

	for _, width := range []int{initMinWidth, 60, 120} {
		model := newInitTeaTestModel(initPromptModel{})
		model.width, model.height = width, 24
		model.transcript = []initTranscriptEntry{{Label: "Template", Value: "Starterskit"}}
		model.Update(initTeaBootstrapDoneMsg{result: initResult{Path: project}})

		view := initView(model)
		for _, line := range strings.Split(view, "\n") {
			if w := lipgloss.Width(line); w > width {
				t.Fatalf("width %d: line too wide (%d): %q", width, w, line)
			}
		}
		plain := ansi.Strip(view)
		if strings.Contains(plain, "…") {
			t.Fatalf("width %d: done footer was truncated:\n%s", width, plain)
		}
		joined := strings.Join(strings.Fields(plain), " ")
		if width >= len(command)+2 && !strings.Contains(plain, command) {
			t.Fatalf("width %d: expected the command on one line, got:\n%s", width, plain)
		}
		if !strings.Contains(strings.ReplaceAll(joined, " ", ""), strings.ReplaceAll(command, " ", "")) {
			t.Fatalf("width %d: command missing from footer:\n%s", width, plain)
		}
	}
}

// A preset that matches nothing is a usage error (exit 2) in the wizard, as
// it is on the line path.
func TestInitTeaUnknownPresetIsUsageError(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.presets = initPresets{Site: "missing", OnConflict: initConflictAsk}
	model.Update(initTeaSitesLoadedMsg{sites: []api.Site{{ID: "s1", Subdomain: "demo"}}})
	if code := exitCode(t, initTeaOutcome(model, nil)); code != ExitUsage {
		t.Fatalf("expected exit %d, got %d", ExitUsage, code)
	}
	var exitErr *ExitError
	if !errors.As(model.err, &exitErr) || !strings.Contains(exitErr.Error(), `site "missing" not found`) {
		t.Fatalf("expected the not-found message, got %v", model.err)
	}
}

// Lipgloss v2 renders colors as given; stdout output outside Bubble Tea must
// be downsampled to what the terminal shows (v1 did this implicitly).
func TestStdoutColorFollowsTerminalProfile(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "")

	if got := stdoutColor(&output.Writer{Color: "never"}, "#ef4444"); got != nil {
		t.Fatalf("expected no color with --color never, got %v", got)
	}
	got := colorAppLogLevel("ERROR", "ERROR", &output.Writer{Color: "always"})
	if strings.Contains(got, "38;2;") {
		t.Fatalf("expected a 256-color sequence on a 256-color terminal, got %q", got)
	}
	if !strings.Contains(got, "38;5;") {
		t.Fatalf("expected the level to be colored, got %q", got)
	}

	t.Setenv("COLORTERM", "truecolor")
	if got := colorAppLogLevel("ERROR", "ERROR", &output.Writer{Color: "always"}); !strings.Contains(got, "38;2;") {
		t.Fatalf("expected 24-bit color on a truecolor terminal, got %q", got)
	}
	if got := colorAppLogLevel("INFO ", "INFO", &output.Writer{Color: "always"}); got != "INFO " {
		t.Fatalf("expected info to stay plain, got %q", got)
	}
}
