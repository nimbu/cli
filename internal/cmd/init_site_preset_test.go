package cmd

import (
	"testing"

	"github.com/nimbu/cli/internal/api"
)

// parseInitPresets parses argv like the real CLI and returns the presets the
// init wizard would see.
func parseInitPresets(t *testing.T, args ...string) initPresets {
	t.Helper()
	parser, cli, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return cli.Init.presets(&cli.RootFlags)
}

// Regression: a NIMBU_SITE exported by direnv used to answer the TTY wizard's
// site step silently. Only --site on the command line (or --env) answers it;
// NIMBU_SITE alone is a default.
func TestInitSitePresetSource(t *testing.T) {
	t.Setenv("NIMBU_SITE", "env-site")

	p := parseInitPresets(t, "init")
	if p.Site != "" || p.DefaultSite != "env-site" {
		t.Fatalf("NIMBU_SITE only: Site=%q DefaultSite=%q, want default only", p.Site, p.DefaultSite)
	}
	if got := p.nonInteractive().Site; got != "env-site" {
		t.Fatalf("non-interactive runs should use NIMBU_SITE, got %q", got)
	}
	if missing := (initPresets{DefaultSite: "env-site", Theme: "t"}).nonInteractive().missingForNoInput(); len(missing) != 0 {
		t.Fatalf("--no-input with NIMBU_SITE and --theme should not miss anything, got %v", missing)
	}

	for _, args := range [][]string{
		{"init", "--site", "cli-site"},
		{"--site=cli-site", "init"},
	} {
		p = parseInitPresets(t, args...)
		if p.Site != "cli-site" || p.DefaultSite != "" {
			t.Fatalf("%v: Site=%q DefaultSite=%q, want explicit cli-site", args, p.Site, p.DefaultSite)
		}
	}

	t.Setenv("NIMBU_SITE", "")
	p = parseInitPresets(t, "init")
	if p.Site != "" || p.DefaultSite != "" {
		t.Fatalf("no site anywhere: Site=%q DefaultSite=%q", p.Site, p.DefaultSite)
	}
}

// --env selects a site explicitly even though it is applied after parsing.
func TestInitSitePresetFromEnvironmentIsExplicit(t *testing.T) {
	cmd := &InitCmd{siteFromEnv: true}
	p := cmd.presets(&RootFlags{Site: "prod-site", Environment: "production"})
	if p.Site != "prod-site" || p.DefaultSite != "" {
		t.Fatalf("--env: Site=%q DefaultSite=%q, want explicit prod-site", p.Site, p.DefaultSite)
	}
}

// The TTY wizard still asks for the site but starts on the NIMBU_SITE one.
func TestInitTeaDefaultSitePreselectsButAsks(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.presets = initPresets{DefaultSite: "demo-shop", OnConflict: initConflictAsk}
	model.Update(initTeaSitesLoadedMsg{sites: []api.Site{
		{ID: "s1", Subdomain: "other"},
		{ID: "s2", Subdomain: "demo-shop", Name: "Demo"},
		{ID: "s3", Subdomain: "third"},
	}})
	if model.step != initTeaStepSite || model.answers.SiteID != "" {
		t.Fatalf("expected the site step to be asked, got step=%s site=%q", model.step, model.answers.SiteID)
	}
	if model.cursor != 1 {
		t.Fatalf("expected cursor on demo-shop (1), got %d", model.cursor)
	}

	// An unknown NIMBU_SITE is no error: the wizard just starts at the top.
	model = newInitTeaTestModel(initPromptModel{})
	model.presets = initPresets{DefaultSite: "gone", OnConflict: initConflictAsk}
	model.Update(initTeaSitesLoadedMsg{sites: []api.Site{{ID: "s1", Subdomain: "other"}}})
	if model.err != nil || model.step != initTeaStepSite || model.cursor != 0 {
		t.Fatalf("unknown default: err=%v step=%s cursor=%d", model.err, model.step, model.cursor)
	}
}
