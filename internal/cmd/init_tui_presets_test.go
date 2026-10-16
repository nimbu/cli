package cmd

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/mod/modfile"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/bootstrap"
	"github.com/nimbu/cli/internal/output"
)

// Flags answer wizard steps up front; the wizard skips straight past them.
func TestInitTeaPresetsSkipAnsweredSteps(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.presets = initPresets{Site: "demo-shop", Theme: "outlet", Repeatables: "all", Bundles: "none", OnConflict: initConflictAsk}
	model.phase = initTeaPhaseLoading

	model.Update(initTeaManifestLoadedMsg{manifest: bootstrap.Manifest{
		Repeatables: []bootstrap.Repeatable{{ID: "header"}, {ID: "text"}},
		Bundles:     []bootstrap.Bundle{{ID: "blog-articles"}},
	}})
	_, cmd := model.Update(initTeaSitesLoadedMsg{sites: []api.Site{{ID: "s1", Subdomain: "other"}, {ID: "s2", Subdomain: "demo-shop", Name: "Demo"}}})
	if cmd == nil || model.answers.SiteID != "s2" || model.phase != initTeaPhaseLoading {
		t.Fatalf("expected --site to pick s2 and load themes, got site=%q phase=%s", model.answers.SiteID, model.phase)
	}

	model.Update(initTeaThemesLoadedMsg{themes: []api.Theme{{ID: "t1", Short: "storefront"}, {ID: "t2", Short: "outlet", Name: "Outlet"}}})
	if model.answers.ThemeID != "t2" || model.step != initTeaStepDirectory {
		t.Fatalf("expected --theme to pick t2 and go to the directory step, got theme=%q step=%s", model.answers.ThemeID, model.step)
	}

	model.handleDirectoryKey(keyEnter())
	if model.step != initTeaStepConfirm {
		t.Fatalf("expected repeatables and bundles presets to skip to confirm, got %s", model.step)
	}
	if got := strings.Join(model.answers.RepeatableIDs, ","); got != "header,text" {
		t.Fatalf("expected all repeatables, got %q", got)
	}
	if len(model.answers.BundleIDs) != 0 {
		t.Fatalf("expected no bundles, got %v", model.answers.BundleIDs)
	}
}

func TestInitTeaPresetErrorsQuitWithMessage(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.presets = initPresets{Site: "missing", OnConflict: initConflictAsk}
	_, cmd := model.Update(initTeaSitesLoadedMsg{sites: []api.Site{{ID: "s1", Subdomain: "demo"}}})
	if cmd == nil || model.err == nil || !strings.Contains(model.err.Error(), `site "missing" not found`) {
		t.Fatalf("expected unknown --site to quit with a clear error, got %v", model.err)
	}
	if err := initTeaOutcome(model, nil); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected outcome to carry the error, got %v", err)
	}
}

// The wizard draws on stderr: its color follows stderr, not stdout.
func TestInitColorProfileFollowsStderrDecision(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")

	if got := initColorProfile(&output.Writer{Color: "never"}); got != colorprofile.ASCII {
		t.Fatalf("expected --color never to pin ASCII, got %v", got)
	}
	if got := initColorProfile(&output.Writer{Color: "always", NoTTY: true}); got == colorprofile.ASCII || got == colorprofile.NoTTY {
		t.Fatalf("expected --color always to keep color on stderr, got %v", got)
	}

	t.Setenv("NO_COLOR", "1")
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Err: &strings.Builder{}, Out: &strings.Builder{}, Color: "auto"})
	if model := newInitTeaModel(ctx, &InitCmd{Dir: "."}, &RootFlags{}); model.useColor {
		t.Fatal("expected NO_COLOR to disable wizard color")
	}
}

// Bubble Tea v1's package init queried the terminal background (OSC 11 + DSR)
// and stalled every nimbu invocation up to 5s on terminals that do not answer.
// Keep the v1 Charm modules out of the build.
func TestCharmV1ModulesStayOutOfTheBuild(t *testing.T) {
	data, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	mod, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		t.Fatalf("parse go.mod: %v", err)
	}
	banned := map[string]bool{
		"github.com/charmbracelet/bubbletea": true,
		"github.com/charmbracelet/bubbles":   true,
		"github.com/charmbracelet/lipgloss":  true,
	}
	for _, req := range mod.Require {
		if banned[req.Mod.Path] {
			t.Fatalf("%s %s is back in go.mod; use the charm.land v2 modules (v1 bubbletea queries the terminal at startup)", req.Mod.Path, req.Mod.Version)
		}
	}
}

func keyEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }
