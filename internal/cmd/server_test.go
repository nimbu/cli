package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerResolveRuntimeConfigFromProjectFile(t *testing.T) {
	root := t.TempDir()
	projectConfig := `site: acme
dev:
  proxy:
    host: 127.0.0.2
    port: 7777
    template_root: theme
    watch: true
    watch_scan_interval: 5s
    max_body_mb: 42
  server:
    command: pnpm
    args:
      - vite
      - --port
      - "5173"
    cwd: apps/web
    ready_url: http://127.0.0.1:5173
    env:
      FOO: BAR
  routes:
    include:
      - POST /.well-known/*
    exclude:
      - GET /assets/*
`
	if err := os.WriteFile(filepath.Join(root, "nimbu.yml"), []byte(projectConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "theme"), 0o755); err != nil {
		t.Fatalf("mkdir theme: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "apps/web"), 0o755); err != nil {
		t.Fatalf("mkdir apps/web: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cmd := &ServerCmd{}
	cfg, warnings, err := cmd.resolveRuntimeConfig()
	if err != nil {
		t.Fatalf("resolve runtime config: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	if cfg.ProxyHost != "127.0.0.2" || cfg.ProxyPort != 7777 {
		t.Fatalf("proxy mismatch: %+v", cfg)
	}
	expectedTemplateRoot := filepath.Join(root, "theme")
	if resolved, err := filepath.EvalSymlinks(expectedTemplateRoot); err == nil {
		expectedTemplateRoot = resolved
	}
	actualTemplateRoot := cfg.TemplateRoot
	if resolved, err := filepath.EvalSymlinks(actualTemplateRoot); err == nil {
		actualTemplateRoot = resolved
	}
	if actualTemplateRoot != expectedTemplateRoot {
		t.Fatalf("template root mismatch: got %q, want %q", actualTemplateRoot, expectedTemplateRoot)
	}
	if cfg.ChildCommand != "pnpm" {
		t.Fatalf("child command mismatch: %q", cfg.ChildCommand)
	}
	if len(cfg.ChildArgs) != 3 {
		t.Fatalf("child args mismatch: %v", cfg.ChildArgs)
	}
	expectedChildCWD := filepath.Join(root, "apps/web")
	if resolved, err := filepath.EvalSymlinks(expectedChildCWD); err == nil {
		expectedChildCWD = resolved
	}
	actualChildCWD := cfg.ChildCWD
	if resolved, err := filepath.EvalSymlinks(actualChildCWD); err == nil {
		actualChildCWD = resolved
	}
	if actualChildCWD != expectedChildCWD {
		t.Fatalf("child cwd mismatch: got %q, want %q", actualChildCWD, expectedChildCWD)
	}
	if cfg.ReadyURL != "http://127.0.0.1:5173" {
		t.Fatalf("ready URL mismatch: %q", cfg.ReadyURL)
	}
	if cfg.MaxBodyMB != 42 {
		t.Fatalf("max body mismatch: %d", cfg.MaxBodyMB)
	}
	if cfg.WatchScanInterval != 5*time.Second {
		t.Fatalf("scan interval mismatch: %s", cfg.WatchScanInterval)
	}
}

func TestServerResolveRuntimeConfigWarnsUnknownKeys(t *testing.T) {
	root := t.TempDir()
	projectConfig := `dev:
  proxy:
    host: 127.0.0.1
    watched: true
  server:
    command: pnpm
    args:
      - vite
`
	if err := os.WriteFile(filepath.Join(root, "nimbu.yml"), []byte(projectConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg, warnings, err := (&ServerCmd{}).resolveRuntimeConfig()
	if err != nil {
		t.Fatalf("resolve runtime config: %v", err)
	}
	if cfg.ChildCommand == "" {
		t.Fatal("child command should still resolve")
	}

	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "dev.proxy") || !strings.Contains(joined, "watched") {
		t.Fatalf("expected unknown key warning, got: %v", warnings)
	}
}

func TestServerResolveRuntimeConfigRequiresChildCommand(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "nimbu.yml"), []byte("dev: {}\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	_, _, err = (&ServerCmd{}).resolveRuntimeConfig()
	if err == nil {
		t.Fatal("expected missing child command error")
	}
}

func TestServerResolveRuntimeConfigRejectsNegativeCLIValues(t *testing.T) {
	root := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	tests := []struct {
		name string
		cmd  ServerCmd
	}{
		{name: "proxy port", cmd: ServerCmd{CMD: "pnpm", ProxyPort: -1}},
		{name: "max body", cmd: ServerCmd{CMD: "pnpm", MaxBodyMB: -1}},
		{name: "watch scan interval", cmd: ServerCmd{CMD: "pnpm", WatchScanInterval: -1 * time.Second}},
		{name: "ready timeout", cmd: ServerCmd{CMD: "pnpm", ReadyTimeout: -1 * time.Second}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, resolveErr := tc.cmd.resolveRuntimeConfig()
			if resolveErr == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestServerResolveRuntimeConfigCLIOverridesInvalidWatchScanInterval(t *testing.T) {
	root := t.TempDir()
	projectConfig := `dev:
  proxy:
    watch_scan_interval: not-a-duration
  server:
    command: pnpm
    args:
      - vite
`
	if err := os.WriteFile(filepath.Join(root, "nimbu.yml"), []byte(projectConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg, _, err := (&ServerCmd{WatchScanInterval: 2 * time.Second}).resolveRuntimeConfig()
	if err != nil {
		t.Fatalf("resolve runtime config: %v", err)
	}
	if cfg.WatchScanInterval != 2*time.Second {
		t.Fatalf("watch scan interval mismatch: %s", cfg.WatchScanInterval)
	}
}

func TestServerResolveRuntimeConfigRejectsNegativeProjectMaxBody(t *testing.T) {
	root := t.TempDir()
	projectConfig := `dev:
  proxy:
    max_body_mb: -1
  server:
    command: pnpm
`
	if err := os.WriteFile(filepath.Join(root, "nimbu.yml"), []byte(projectConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	_, _, err = (&ServerCmd{}).resolveRuntimeConfig()
	if err == nil {
		t.Fatal("expected invalid dev.proxy.max_body_mb error")
	}
}
