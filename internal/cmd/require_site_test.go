package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/config"
)

func requireSiteContext(t *testing.T, flagSite, defaultSite string) context.Context {
	t.Helper()
	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, &RootFlags{Site: flagSite})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{DefaultSite: defaultSite})
	return ctx
}

func writeProjectSite(t *testing.T, dir, site string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, config.ProjectFileName)
	if err := os.WriteFile(path, []byte("site: "+site+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRequireSiteUsesProjectFileFromParent(t *testing.T) {
	t.Setenv(config.ProjectDirEnv, "")

	root := t.TempDir()
	nested := filepath.Join(root, "scratch", "agent")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectSite(t, root, "parent-site")
	t.Chdir(nested)

	got, err := RequireSite(requireSiteContext(t, "", ""), "")
	if err != nil {
		t.Fatalf("RequireSite: %v", err)
	}
	if got != "parent-site" {
		t.Fatalf("RequireSite() = %q, want parent-site", got)
	}
}

func TestRequireSitePrecedence(t *testing.T) {
	t.Setenv(config.ProjectDirEnv, "")

	root := t.TempDir()
	writeProjectSite(t, root, "yml-site")
	t.Chdir(root)

	// Explicit --site wins over config default and nimbu.yml.
	got, err := RequireSite(requireSiteContext(t, "flag-site", "config-site"), "")
	if err != nil {
		t.Fatalf("RequireSite: %v", err)
	}
	if got != "flag-site" {
		t.Fatalf("RequireSite() = %q, want flag-site", got)
	}

	// A site already resolved upstream wins over all.
	got, err = RequireSite(requireSiteContext(t, "flag-site", "config-site"), "resolved-site")
	if err != nil {
		t.Fatalf("RequireSite: %v", err)
	}
	if got != "resolved-site" {
		t.Fatalf("RequireSite() = %q, want resolved-site", got)
	}

	// Inside a project, nimbu.yml beats the global default_site.
	got, err = RequireSite(requireSiteContext(t, "", "config-site"), "")
	if err != nil {
		t.Fatalf("RequireSite: %v", err)
	}
	if got != "yml-site" {
		t.Fatalf("RequireSite() = %q, want yml-site", got)
	}
}

func TestResolveSitePrecedencePairs(t *testing.T) {
	type sources struct {
		env, flag, project, config string
	}
	tests := []struct {
		name       string
		in         sources
		wantSite   string
		wantSource string
	}{
		{"env beats flag", sources{env: "env-site", flag: "flag-site"}, "env-site", siteSourceEnvironment},
		{"env beats project", sources{env: "env-site", project: "yml-site"}, "env-site", siteSourceEnvironment},
		{"env beats config", sources{env: "env-site", config: "config-site"}, "env-site", siteSourceEnvironment},
		{"flag beats project", sources{flag: "flag-site", project: "yml-site"}, "flag-site", siteSourceFlag},
		{"flag beats config", sources{flag: "flag-site", config: "config-site"}, "flag-site", siteSourceFlag},
		{"project beats config", sources{project: "yml-site", config: "config-site"}, "yml-site", siteSourceProject},
		{"config alone", sources{config: "config-site"}, "config-site", siteSourceConfig},
		{"project alone", sources{project: "yml-site"}, "yml-site", siteSourceProject},
		{"nothing", sources{}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubProjectSite(t, tt.in.project, nil)
			flags := &RootFlags{Site: tt.in.flag}
			if tt.in.env != "" {
				// applyEnvironment overwrites Site with the environment's site.
				flags.Site, flags.Environment = tt.in.env, "production"
			}
			got := resolveSite(flags, &config.Config{DefaultSite: tt.in.config})
			if got.Site != tt.wantSite || got.Source != tt.wantSource {
				t.Fatalf("resolveSite() = %q (%s), want %q (%s)", got.Site, got.Source, tt.wantSite, tt.wantSource)
			}
		})
	}
}

func TestResolveSiteEnvironmentFlagOverridesExplicitSite(t *testing.T) {
	environmentProject(t)
	flags := &RootFlags{Environment: "production", Site: "explicit-site"}
	if err := applyEnvironment(flags, nil); err != nil {
		t.Fatal(err)
	}
	got := resolveSite(flags, &config.Config{DefaultSite: "config-site"})
	if got.Site != "prod-site" || got.Source != siteSourceEnvironment {
		t.Fatalf("resolveSite() = %+v, want prod-site from environment", got)
	}
}

func TestResolveSiteNimbuSiteEnvBeatsProject(t *testing.T) {
	stubProjectSite(t, "yml-site", nil)
	t.Setenv("NIMBU_SITE", "env-var-site")
	parser, cli, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"sites", "current"}); err != nil {
		t.Fatal(err)
	}
	got := resolveSite(&cli.RootFlags, &config.Config{DefaultSite: "config-site"})
	if got.Site != "env-var-site" || got.Source != siteSourceFlag {
		t.Fatalf("resolveSite() = %+v, want env-var-site from NIMBU_SITE", got)
	}
}

func TestResolveSiteToleratesMissingContext(t *testing.T) {
	stubProjectSite(t, "", config.ErrNotFound)
	if got := resolveSite(nil, nil); got.Site != "" || got.ProjectErr == nil {
		t.Fatalf("resolveSite(nil, nil) = %+v", got)
	}
	_, err := RequireSite(context.Background(), "")
	desc := classifyError(err)
	if desc.Code != errorSiteRequired || desc.ExitCode != ExitUsage {
		t.Fatalf("missing site classified as %s exit %d, want %s exit %d", desc.Code, desc.ExitCode, errorSiteRequired, ExitUsage)
	}
}

func stubProjectSite(t *testing.T, site string, err error) {
	t.Helper()
	original := readProjectSite
	readProjectSite = func() (string, error) { return site, err }
	t.Cleanup(func() { readProjectSite = original })
}

func TestRequireSiteErrorNamesSearchBoundary(t *testing.T) {
	t.Setenv(config.ProjectDirEnv, "")

	repo := t.TempDir()
	nested := filepath.Join(repo, "scratch")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)

	_, err := RequireSite(requireSiteContext(t, "", ""), "")
	if err == nil {
		t.Fatal("expected error")
	}
	desc := classifyError(err)
	if desc.Code != errorSiteRequired || desc.ExitCode != ExitUsage {
		t.Fatalf("classified as %s exit %d, want %s exit %d", desc.Code, desc.ExitCode, errorSiteRequired, ExitUsage)
	}
	if desc.Message != "no site selected" {
		t.Fatalf("message %q", desc.Message)
	}
	if !strings.Contains(desc.Hint, "--site") || !strings.Contains(desc.Hint, "nimbu.yml (searched up to "+repo+")") {
		t.Fatalf("hint %q missing --site or search boundary", desc.Hint)
	}
}

func TestResolveSiteBrokenProjectFileDoesNotFallBackToDefault(t *testing.T) {
	stubProjectSite(t, "", errors.New("yaml: line 2: did not find expected key"))
	got := resolveSite(&RootFlags{}, &config.Config{DefaultSite: "config-site"})
	if got.Site != "" {
		t.Fatalf("resolveSite() = %q; a broken nimbu.yml must not fall back to default_site", got.Site)
	}
	desc := classifyError(siteRequiredError(got.ProjectErr))
	if desc.Code != errorSiteRequired || !strings.Contains(desc.Message, "cannot read the site from nimbu.yml") || !strings.Contains(desc.Hint, "--site") {
		t.Fatalf("desc = %+v", desc)
	}
	// An explicit --site still works with a broken project file.
	if got := resolveSite(&RootFlags{Site: "flag-site"}, nil); got.Site != "flag-site" {
		t.Fatalf("resolveSite() = %q", got.Site)
	}
}
