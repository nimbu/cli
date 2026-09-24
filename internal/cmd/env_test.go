package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func environmentProject(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(config.ProjectDirEnv, dir)
	if err := os.WriteFile(filepath.Join(dir, "nimbu.yml"), []byte("site: default-site\nenvironments:\n  production: {host: api.production.test, site: prod-site, protected: true}\n  staging: {host: api.staging.test, site: stage-site}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEnvironmentOverridesExplicitHostAndSite(t *testing.T) {
	environmentProject(t)
	flags := RootFlags{Environment: "production", Site: "explicit-site", APIURL: "https://api.explicit.test"}
	if err := applyEnvironment(&flags); err != nil {
		t.Fatal(err)
	}
	if flags.Site != "prod-site" || flags.APIURL != "https://api.production.test" || !IsProtectedEnvironment(&flags) {
		t.Fatalf("flags = %#v", flags)
	}
	flags.Environment = "unknown"
	if err := applyEnvironment(&flags); err == nil || !strings.Contains(err.Error(), "unknown environment") {
		t.Fatalf("error = %v", err)
	}
}

func TestEnvironmentReferenceResolution(t *testing.T) {
	environmentProject(t)
	ctx := context.WithValue(context.Background(), rootFlagsKey{}, &RootFlags{Site: "default", APIURL: "https://api.default.test"})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})
	site, err := parseSiteRefForCommand(ctx, "production", "https://api.explicit.test")
	if err != nil {
		t.Fatal(err)
	}
	if site.Site != "prod-site" || site.BaseURL != "https://api.production.test" {
		t.Fatalf("site = %#v", site)
	}
	channel, err := parseChannelRefForCommand(ctx, "staging/news", "")
	if err != nil {
		t.Fatal(err)
	}
	if channel.Site != "stage-site" || channel.Channel != "news" || channel.BaseURL != "https://api.staging.test" {
		t.Fatalf("channel = %#v", channel)
	}
	theme, err := parseThemeCopyRef(ctx, "production/default-theme", "")
	if err != nil {
		t.Fatal(err)
	}
	if theme.Site != "prod-site" || theme.BaseURL != "https://api.production.test" {
		t.Fatalf("theme = %#v", theme)
	}
	legacy, err := parseChannelRefForCommand(ctx, "other-site/news", "")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Site != "other-site" || legacy.BaseURL != "https://api.default.test" {
		t.Fatalf("legacy = %#v", legacy)
	}
}

func TestProtectedEnvironmentRequiresYesEvenWithForce(t *testing.T) {
	flags := &RootFlags{Environment: "production", SelectedEnvironmentProtected: true, NoInput: true, Force: true}
	err := confirmProtectedEnvironment(context.Background(), flags, false, "apply")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %v", err)
	}
	if err := confirmProtectedEnvironment(context.Background(), flags, true, "apply"); err != nil {
		t.Fatal(err)
	}
	flags.SelectedEnvironmentProtected = false
	if err := confirmProtectedEnvironment(context.Background(), flags, false, "apply"); err != nil {
		t.Fatal(err)
	}
}

func TestEnvCommandGrammar(t *testing.T) {
	parser, cli, err := newParser()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"env", "add", "production", "--host", "api.nimbu.io", "--site", "prod", "--protected"}); err != nil {
		t.Fatal(err)
	}
	if cli.Site != "prod" || cli.Env.Add.Name != "production" || !cli.Env.Add.Protected {
		t.Fatalf("add = %#v", cli.Env.Add)
	}
}

func TestEnvironmentCommandsPersistAndList(t *testing.T) {
	environmentProject(t)
	var out bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: &out, Err: &out})
	ctx = output.WithMode(ctx, output.Mode{JSON: true})
	flags := &RootFlags{Site: "test-site"}
	add := &EnvAddCmd{Name: "testing", Host: "api.testing.test"}
	if err := add.Run(ctx, flags); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := (&EnvListCmd{}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "testing"`) || !strings.Contains(out.String(), `"site": "test-site"`) {
		t.Fatalf("list = %s", out.String())
	}
	if err := (&EnvRemoveCmd{Name: "testing"}).Run(ctx, flags); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.ReadProjectConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := cfg.Environments["testing"]; exists {
		t.Fatal("environment not removed")
	}
	flags.Readonly = true
	if err := add.Run(ctx, flags); err == nil {
		t.Fatal("expected readonly error")
	}
}

func TestEnvironmentNameDoesNotOverrideBareChannelReference(t *testing.T) {
	environmentProject(t)
	ctx := context.WithValue(context.Background(), rootFlagsKey{}, &RootFlags{Site: "default", APIURL: "https://api.default.test"})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})
	for _, raw := range []string{"production", "/production/"} {
		channel, err := parseChannelRefForCommand(ctx, raw, "https://api.explicit.test")
		if err != nil {
			t.Fatal(err)
		}
		if channel.Channel != "production" || channel.Site != "default" || channel.BaseURL != "https://api.explicit.test" {
			t.Fatalf("channel = %#v", channel)
		}
	}
}
