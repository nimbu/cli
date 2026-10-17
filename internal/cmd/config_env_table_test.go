package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func isolatedUserConfig(t *testing.T, body string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestConfigListPlainIsOrderedAndEscaped(t *testing.T) {
	isolatedUserConfig(t, `{"default_site": "tab\there", "api_url": "https://api.example.test"}`)
	var out bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: &out, Err: discardWriter{}, NoTTY: true})
	ctx = output.WithMode(ctx, output.Mode{Plain: true})
	for i := 0; i < 5; i++ { // map iteration used to shuffle rows between runs
		out.Reset()
		if err := (&ConfigListCmd{}).Run(ctx); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
		keys := make([]string, 0, len(lines))
		for _, line := range lines {
			fields := strings.Split(line, "\t")
			if len(fields) != 2 {
				t.Fatalf("row %q: want 2 fields", line)
			}
			keys = append(keys, fields[0])
		}
		if got := strings.Join(keys, ","); got != strings.Join(configListKeys, ",") {
			t.Fatalf("keys = %s", got)
		}
		if !strings.HasPrefix(out.String(), "default_site\ttab\\there\n") {
			t.Fatalf("default_site row not escaped: %q", out.String())
		}
	}
}

func TestConfigListTableAlignsWideValues(t *testing.T) {
	isolatedUserConfig(t, `{"default_site": "日本語サイト"}`)
	var out bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: &out, Err: discardWriter{}, NoTTY: true})
	ctx = output.WithMode(ctx, output.Mode{})
	if err := (&ConfigListCmd{}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	assertValueColumnAligned(t, out.String(), "VALUE")
}

func TestEnvListTableAlignsWideValues(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.ProjectDirEnv, dir)
	yml := "site: default-site\nenvironments:\n  プロダクション: {host: api.production.test, site: prod-site, protected: true}\n  staging: {host: api.staging.test, site: stage-site}\n"
	if err := os.WriteFile(filepath.Join(dir, "nimbu.yml"), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: &out, Err: discardWriter{}, NoTTY: true})
	ctx = output.WithMode(ctx, output.Mode{})
	if err := (&EnvListCmd{}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	assertValueColumnAligned(t, out.String(), "HOST")

	out.Reset()
	ctx = output.WithMode(ctx, output.Mode{Plain: true})
	if err := (&EnvListCmd{}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	want := "staging\tapi.staging.test\tstage-site\tfalse\nプロダクション\tapi.production.test\tprod-site\ttrue\n"
	if out.String() != want {
		t.Fatalf("plain = %q, want %q", out.String(), want)
	}
}

// assertValueColumnAligned checks that the column headed by header starts at
// the same display cell on every row (rune-width tabwriter got this wrong for
// CJK text).
func assertValueColumnAligned(t *testing.T, text, header string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected a header and rows, got %q", text)
	}
	col := strings.Index(lines[0], header)
	if col < 0 {
		t.Fatalf("header %q missing in %q", header, lines[0])
	}
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue // empty value (e.g. unset keyring_backend)
		}
		idx := strings.Index(line, " "+fields[1]) + 1
		if got := ansi.StringWidth(line[:idx]); got != col {
			t.Fatalf("column %s starts at cell %d, want %d:\n%s", header, got, col, text)
		}
	}
}
