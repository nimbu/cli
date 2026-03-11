package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/bootstrap"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestInitTeaViewShowsVisibleSiteFilter(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{
		Sites: []initSiteChoice{
			{Label: "Zorgpoort (zorgpoort)"},
			{Label: "Zenjoy (zenjoy)"},
		},
	})
	model.width = 72
	model.height = 24
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepSite
	model.filterInput.SetValue("zorg")

	view := model.View()

	for _, needle := range []string{"Search", "zorg", "Zorgpoort"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("expected view to contain %q, got:\n%s", needle, view)
		}
	}
}

func TestInitTeaViewKeepsTranscriptVisibleAcrossSteps(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{
		Themes: []initThemeChoice{
			{Label: "Storefront (storefront)"},
		},
		DefaultDirectoryName: "theme-demo-shop",
		OutputDir:            "/tmp/output",
		Source:               "zenjoy/theme-starterskit@vite-go-cli",
	})
	model.width = 84
	model.height = 24
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepDirectory
	model.transcript = []initTranscriptEntry{
		{Label: "Site", Value: "Demo Shop (demo-shop)"},
		{Label: "Theme", Value: "Storefront (storefront)"},
	}
	model.directoryInput.SetValue("theme-demo-shop")

	view := model.View()

	for _, needle := range []string{"Site", "Demo Shop", "Theme", "Storefront", "theme-demo-shop"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("expected view to contain %q, got:\n%s", needle, view)
		}
	}
	if strings.Contains(view, "Directory: choose a directory name") {
		t.Fatalf("expected directory step to avoid separate helper line, got:\n%s", view)
	}
	if !strings.Contains(view, "Directory: theme-demo-shop") {
		t.Fatalf("expected single directory input row, got:\n%s", view)
	}
}

func TestInitTeaRepeatablesModeOmitsSearchAndUsesCopyPrompt(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.width = 84
	model.height = 24
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepRepeatableMode

	view := model.View()

	if !strings.Contains(view, "Repeatables: which to copy?") {
		t.Fatalf("expected updated repeatables mode prompt, got:\n%s", view)
	}
	if strings.Contains(view, "Search:") {
		t.Fatalf("expected repeatables mode to omit search line, got:\n%s", view)
	}
}

func TestInitTeaConfirmStepHighlightsFinalAction(t *testing.T) {
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   &strings.Builder{},
		Err:   &strings.Builder{},
		Color: "always",
	})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})

	model := newInitTeaModel(ctx, &InitCmd{Dir: "."}, &RootFlags{})
	model.width = 120
	model.height = 24
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepConfirm
	model.outputDir = "/tmp/output"
	model.answers.DirectoryName = "theme-demo-shop"
	model.answers.ThemeID = "theme-1"
	model.answers.RepeatableIDs = []string{"header"}
	model.answers.BundleIDs = []string{"blog-articles"}

	view := model.View()

	if !strings.Contains(view, "Enter create project, Esc cancel") ||
		!strings.Contains(view, "\x1b[1;38;2;96;165;250mEnter create project, Esc cancel\x1b[0m") ||
		!strings.Contains(view, "│\x1b[0m \n") {
		t.Fatalf("expected blank spacer and emphasized final action line, got:\n%s", view)
	}
}

func TestInitTeaIntroUsesDimCornerAndBrightText(t *testing.T) {
	view := renderInitTeaViewForTheme(t, "")

	if !strings.Contains(view, "\x1b[38;2;147;163;184m┌\x1b[0m") {
		t.Fatalf("expected intro corner to use dim rail color, got:\n%s", view)
	}
	if !strings.Contains(view, "\x1b[1;38;2;226;232;240mLet's setup a new project.\x1b[0m") {
		t.Fatalf("expected intro text to stay bright, got:\n%s", view)
	}
}

func TestInitTeaSelectedRepeatableMarkerUsesBlueAccent(t *testing.T) {
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   &strings.Builder{},
		Err:   &strings.Builder{},
		Color: "always",
	})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})

	model := newInitTeaModel(ctx, &InitCmd{Dir: "."}, &RootFlags{})
	model.width = 84
	model.height = 24
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepRepeatables
	model.prompt.RepeatableOptions = []bootstrap.Repeatable{
		{ID: "header", Label: "Header"},
		{ID: "text", Label: "Text"},
	}
	model.repeatables["header"] = struct{}{}

	view := model.View()

	if !strings.Contains(view, "› \x1b[1;38;2;96;165;250m●\x1b[0m Header") {
		t.Fatalf("expected selected repeatable marker to use blue accent color, got:\n%s", view)
	}
}

func TestInitTeaBundlesStepOmitsSearch(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{
		BundleOptions: []bootstrap.Bundle{
			{ID: "customer-auth", Label: "Customer login with password reset"},
			{ID: "blog-articles", Label: "Blog + articles"},
		},
	})
	model.width = 84
	model.height = 24
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepBundles

	view := model.View()

	if strings.Contains(view, "Search:") {
		t.Fatalf("expected functional areas step to omit search line, got:\n%s", view)
	}
}

func TestInitTeaSelectedBundleMarkerUsesBlueAccent(t *testing.T) {
	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   &strings.Builder{},
		Err:   &strings.Builder{},
		Color: "always",
	})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})

	model := newInitTeaModel(ctx, &InitCmd{Dir: "."}, &RootFlags{})
	model.width = 84
	model.height = 24
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepBundles
	model.prompt.BundleOptions = []bootstrap.Bundle{
		{ID: "customer-auth", Label: "Customer login with password reset"},
		{ID: "blog-articles", Label: "Blog + articles"},
	}
	model.bundles["customer-auth"] = struct{}{}

	view := model.View()

	if !strings.Contains(view, "› \x1b[1;38;2;96;165;250m●\x1b[0m Customer login with password reset") {
		t.Fatalf("expected selected bundle marker to use blue accent color, got:\n%s", view)
	}
}

func TestInitTeaViewUsesTimelineLayoutInsteadOfPanels(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{
		Sites: []initSiteChoice{
			{Label: "Demo Shop (demo-shop)"},
			{Label: "Speeltuin Test (speeltuin-test)"},
		},
		Source: "zenjoy/theme-starterskit@vite-go-cli",
	})
	model.width = 84
	model.height = 24
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepSite
	model.transcript = []initTranscriptEntry{
		{Label: "Source", Value: "zenjoy/theme-starterskit@vite-go-cli"},
		{Label: "Repository", Value: "cloned"},
	}

	view := model.View()

	for _, needle := range []string{"│", "◇", "●", "Site: choose a site"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("expected timeline view to contain %q, got:\n%s", needle, view)
		}
	}
	if strings.Contains(view, "\n  │") || strings.Contains(view, "\n  ◇") || strings.Contains(view, "\n  ●") {
		t.Fatalf("expected timeline to be flush-left without leading spaces, got:\n%s", view)
	}
	for _, unwanted := range []string{"╭", "╰", "╮", "╯"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("expected timeline layout without boxed panels, found %q in:\n%s", unwanted, view)
		}
	}
}

func TestInitTeaViewShowsLoadingState(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.width = 84
	model.height = 24
	model.phase = initTeaPhaseLoading
	model.loadingSummary = "Template"
	model.loadingDetail = "Cloning repository..."

	view := model.View()

	for _, needle := range []string{"Template", "Cloning repository...", "nimbu"} {
		if !strings.Contains(strings.ToLower(view), strings.ToLower(needle)) {
			t.Fatalf("expected view to contain %q, got:\n%s", needle, view)
		}
	}
	if strings.Contains(view, "\n init \n") {
		t.Fatalf("expected standalone init chip to be removed, got:\n%s", view)
	}
	if !strings.Contains(view, "Let's setup a new project.") {
		t.Fatalf("expected intro line below banner, got:\n%s", view)
	}
	if !strings.Contains(view, "┌   Let's setup a new project.\n│\n") {
		t.Fatalf("expected intro line to hand off into the timeline rail, got:\n%s", view)
	}
	if !strings.Contains(view, model.spinner.View()) {
		t.Fatalf("expected inline spinner in loading row, got:\n%s", view)
	}
	if strings.Contains(view, "│ Cloning repository...") {
		t.Fatalf("expected loading status to stay on a single row, got:\n%s", view)
	}
}

func TestInitTeaViewRespectsNarrowWidth(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{
		Sites: []initSiteChoice{
			{Label: "Doelzoeker staging environment for the long customer name (doelzoeker-staging)"},
			{Label: "Another extremely long customer label for wrapping checks (another-staging)"},
		},
		BundleOptions: []bootstrap.Bundle{
			{ID: "blog-articles", Label: "Blog + articles"},
		},
	})
	model.width = 48
	model.height = 18
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepSite
	model.filterInput.SetValue("doel")

	view := model.View()

	for _, line := range strings.Split(view, "\n") {
		if len([]rune(line)) > model.width {
			t.Fatalf("line exceeds width %d: %q\nfull view:\n%s", model.width, line, view)
		}
	}
}

func TestInitTeaLoadingViewUsesTranscriptStyle(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{
		Source: "zenjoy/theme-starterskit@vite-go-cli",
	})
	model.width = 84
	model.height = 24
	model.phase = initTeaPhaseLoading
	model.loadingSummary = "Template"
	model.loadingDetail = "Loading template manifest..."
	model.transcript = []initTranscriptEntry{
		{Text: "Found 41 themes"},
	}

	view := model.View()

	for _, needle := range []string{"◇", "Template: Loading template manifest...", model.spinner.View()} {
		if !strings.Contains(view, needle) {
			t.Fatalf("expected loading transcript to contain %q, got:\n%s", needle, view)
		}
	}
	if strings.Contains(view, "\n│ "+model.spinner.View()) || strings.Contains(view, "\n● ") {
		t.Fatalf("expected single-line spinner bullet loading row, got:\n%s", view)
	}
	if strings.Contains(view, "╭") {
		t.Fatalf("expected loading view without rounded panels, got:\n%s", view)
	}
}

func TestInitTeaPreflightCollapsesIntoOneSummaryRow(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.phase = initTeaPhaseLoading
	model.loadingSummary = "Template"
	model.loadingDetail = "Cloning repository..."

	if _, cmd := model.Update(initTeaSourceResolvedMsg{
		sourceDir:   "/tmp/source",
		sourceLabel: "zenjoy/theme-starterskit@vite-go-cli",
		outputDir:   "/tmp/out",
		cleanup:     func() {},
	}); cmd == nil {
		// no-op, state mutation is what matters
	}
	if _, cmd := model.Update(initTeaManifestLoadedMsg{manifest: bootstrap.Manifest{Name: "Starterskit"}}); cmd == nil {
	}
	if got := len(model.transcript); got != 1 {
		t.Fatalf("expected template transcript row immediately after manifest load, got %#v", model.transcript)
	}
	if got := model.transcript[0]; got.Label != "Template" || got.Value != "Starterskit" {
		t.Fatalf("expected immediate template transcript row, got %#v", got)
	}
	if model.loadingSummary != "Site" || model.loadingDetail != "Fetching your sites..." {
		t.Fatalf("expected active site loading row after manifest load, got summary=%q detail=%q", model.loadingSummary, model.loadingDetail)
	}
	if _, cmd := model.Update(initTeaSitesLoadedMsg{
		sites: []api.Site{{ID: "site-1", Name: "Demo Shop", Subdomain: "demo-shop"}},
	}); cmd == nil {
	}

	if got := len(model.transcript); got != 1 {
		t.Fatalf("expected template row only after sites load, got %d entries: %#v", got, model.transcript)
	}
	if strings.Contains(model.transcript[0].Text, "Source") || strings.Contains(model.transcript[0].Text, "manifest") {
		t.Fatalf("expected no detailed preflight substeps in final transcript, got %#v", model.transcript[0])
	}
	if strings.Contains(model.View(), "Found 1 sites") {
		t.Fatalf("expected site count to stay out of transcript once selection begins, got:\n%s", model.View())
	}
}

func TestInitTeaManifestLoadedShowsActiveSiteFetchingRow(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.width = 84
	model.height = 24
	model.phase = initTeaPhaseLoading
	model.sourceLabel = "zenjoy/theme-starterskit@vite-go-cli"

	updated, _ := model.Update(initTeaManifestLoadedMsg{manifest: bootstrap.Manifest{Name: "Starterskit"}})
	model = updated.(*initTeaModel)

	view := model.View()
	for _, needle := range []string{"Let's setup a new project.", "Template: Starterskit", "Site: Fetching your sites..."} {
		if !strings.Contains(view, needle) {
			t.Fatalf("expected view to contain %q, got:\n%s", needle, view)
		}
	}
}

func TestInitTeaSiteSelectionCollapsesSiteCountIntoSingleRow(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepSite
	model.sites = []api.Site{{ID: "site-1", Name: "Demo Shop", Subdomain: "demo-shop"}}
	model.prompt.Sites = initSiteChoices(model.sites)
	model.transcript = []initTranscriptEntry{{Label: "Template", Value: "Starterskit"}}

	if _, cmd := model.confirmCurrentSelection(); cmd == nil {
		t.Fatalf("expected site confirmation to trigger theme loading")
	}

	if got := len(model.transcript); got != 2 {
		t.Fatalf("expected starterskit summary plus selected site row, got %#v", model.transcript)
	}
	if got := model.transcript[1]; got.Label != "Site" || got.Value != "Demo Shop (demo-shop)" {
		t.Fatalf("expected collapsed site row, got %#v", got)
	}
}

func TestInitTeaSingleThemeAutoSelectsAndSkipsThemeStep(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.phase = initTeaPhaseLoading
	model.step = initTeaStepSite
	model.sites = []api.Site{{ID: "site-1", Name: "Demo Shop", Subdomain: "demo-shop"}}
	model.prompt.Sites = initSiteChoices(model.sites)
	model.answers.SiteID = "site-1"
	model.appendTranscript("Site", "Demo Shop (demo-shop)")

	updated, _ := model.Update(initTeaThemesLoadedMsg{
		themes: []api.Theme{{ID: "theme-1", Name: "Storefront"}},
	})
	model = updated.(*initTeaModel)

	if model.step != initTeaStepDirectory {
		t.Fatalf("expected single theme to skip theme step and go to directory, got %s", model.step)
	}
	if model.answers.ThemeID != "theme-1" {
		t.Fatalf("expected single theme to be selected automatically, got %q", model.answers.ThemeID)
	}
	if got := model.transcript[len(model.transcript)-1]; got.Label != "Theme" || !strings.Contains(got.Value, "Storefront") {
		t.Fatalf("expected theme transcript row, got %#v", got)
	}
	if strings.Contains(model.View(), "Found 1 themes") {
		t.Fatalf("expected theme count to stay out of transcript when auto-selecting, got:\n%s", model.View())
	}
}

func TestInitTeaLoadingSitesIsSeparateFromStarterskitLoading(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.phase = initTeaPhaseLoading

	updated, _ := model.Update(initTeaManifestLoadedMsg{})
	model = updated.(*initTeaModel)
	if model.loadingSummary != "Site" || model.loadingDetail != "Fetching your sites..." {
		t.Fatalf("expected separate site-loading phase, got summary=%q detail=%q", model.loadingSummary, model.loadingDetail)
	}
}

func TestInitTeaTemplateFallbackUsesRepoAndBranchSourceLabel(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.sourceLabel = "zenjoy/theme-starterskit@vite-go-cli"

	if got := model.templateDisplayName(); got != "zenjoy/theme-starterskit#vite-go-cli" {
		t.Fatalf("expected repo#branch fallback, got %q", got)
	}
}

func TestInitTeaSitePromptUsesFieldLabelCopy(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{
		Sites: []initSiteChoice{{Label: "Demo Shop (demo-shop)"}},
	})
	model.width = 84
	model.height = 24
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepSite

	view := model.View()

	if !strings.Contains(view, "Site: choose a site") {
		t.Fatalf("expected field-style site prompt, got:\n%s", view)
	}
}

func TestInitTeaTranscriptUsesDimLabelAndBrightValue(t *testing.T) {
	view := renderInitTeaViewForTheme(t, "")
	if !strings.Contains(view, "\x1b[") {
		t.Fatalf("expected ANSI-colored init view, got:\n%s", view)
	}

	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   &strings.Builder{},
		Err:   &strings.Builder{},
		Color: "always",
	})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})

	model := newInitTeaModel(ctx, &InitCmd{Dir: "."}, &RootFlags{})
	model.width = 120
	model.height = 24
	model.phase = initTeaPhasePrompt
	model.step = initTeaStepDirectory
	model.transcript = []initTranscriptEntry{{Label: "Site", Value: "Demo Shop (demo-shop)"}}
	model.directoryInput.SetValue("theme-demo-shop")

	view = model.View()
	if !strings.Contains(view, "Site") || !strings.Contains(view, "Demo Shop") {
		t.Fatalf("expected transcript content in view, got:\n%s", view)
	}
	if !strings.Contains(view, "\x1b[") {
		t.Fatalf("expected styled label/value transcript row, got:\n%s", view)
	}
}

func TestInitTeaViewUsesConfiguredBannerTheme(t *testing.T) {
	defaultView := renderInitTeaViewForTheme(t, "")
	rainbowView := renderInitTeaViewForTheme(t, "rainbow")

	if defaultView == rainbowView {
		t.Fatalf("expected themed banner output to differ from default")
	}
	if !strings.Contains(rainbowView, "\x1b[") {
		t.Fatalf("expected themed banner to use ANSI colors, got:\n%s", rainbowView)
	}
}

func renderInitTeaViewForTheme(t *testing.T, theme string) string {
	t.Helper()

	cfg := config.Defaults()
	cfg.BannerTheme = theme

	ctx := output.WithMode(context.Background(), output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   &strings.Builder{},
		Err:   &strings.Builder{},
		Color: "always",
	})
	ctx = context.WithValue(ctx, configKey{}, &cfg)

	model := newInitTeaModel(ctx, &InitCmd{Dir: "."}, &RootFlags{})
	model.width = 120
	model.height = 24
	model.phase = initTeaPhaseLoading
	model.loadingSummary = "Template"
	model.loadingDetail = "Loading template manifest..."
	return model.View()
}
