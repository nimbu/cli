package cmd

import (
	"context"
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

	// A site already resolved upstream (flag or NIMBU_SITE) wins over all.
	got, err = RequireSite(requireSiteContext(t, "flag-site", "config-site"), "resolved-site")
	if err != nil {
		t.Fatalf("RequireSite: %v", err)
	}
	if got != "resolved-site" {
		t.Fatalf("RequireSite() = %q, want resolved-site", got)
	}

	// Config default wins over nimbu.yml.
	got, err = RequireSite(requireSiteContext(t, "", "config-site"), "")
	if err != nil {
		t.Fatalf("RequireSite: %v", err)
	}
	if got != "config-site" {
		t.Fatalf("RequireSite() = %q, want config-site", got)
	}
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
	want := "site required; use --site flag, NIMBU_SITE env, or nimbu.yml (searched up to " + repo + ")"
	if err.Error() != want {
		t.Fatalf("error %q, want %q", err, want)
	}
	if !strings.Contains(err.Error(), "searched up to") {
		t.Fatalf("error %q missing search boundary", err)
	}
}
