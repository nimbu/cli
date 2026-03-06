package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadProjectConfigFromYAMLDocument(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ProjectFileName)
	data := `---
site: acme
theme: storefront
dev:
  proxy:
    watch_scan_interval: 3s
  server:
    command: yarn
    args:
      - dev:server
`

	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := ReadProjectConfigFrom(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	if cfg.Site != "acme" {
		t.Fatalf("site mismatch: %q", cfg.Site)
	}
	if cfg.Theme != "storefront" {
		t.Fatalf("theme mismatch: %q", cfg.Theme)
	}
	if cfg.Dev == nil {
		t.Fatal("expected dev config")
	}
	if cfg.Dev.Proxy.WatchScanInterval != "3s" {
		t.Fatalf("watch scan interval mismatch: %q", cfg.Dev.Proxy.WatchScanInterval)
	}
	if cfg.Dev.Server.Command != "yarn" {
		t.Fatalf("server command mismatch: %q", cfg.Dev.Server.Command)
	}
}

func TestWarnUnknownDevKeysYAML(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ProjectFileName)
	data := `dev:
  proxy:
    host: 127.0.0.1
    watched: true
  server:
    command: yarn
    args:
      - dev:server
  routes:
    include:
      - POST /.well-known/*
    ignored: true
`

	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	warnings, err := WarnUnknownDevKeys(path)
	if err != nil {
		t.Fatalf("warn unknown dev keys: %v", err)
	}

	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "dev.proxy") || !strings.Contains(joined, "watched") {
		t.Fatalf("missing proxy warning: %v", warnings)
	}
	if !strings.Contains(joined, "dev.routes") || !strings.Contains(joined, "ignored") {
		t.Fatalf("missing routes warning: %v", warnings)
	}
}

func TestReadProjectConfigSyncYAML(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ProjectFileName)
	data := `site: acme
theme: storefront
sync:
  build:
    command: yarn
    args:
      - build
  roots:
    assets:
      - public
  generated:
    - dist/**
`

	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := ReadProjectConfigFrom(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	if cfg.Sync == nil {
		t.Fatal("expected sync config")
	}
	if cfg.Sync.Build.Command != "yarn" {
		t.Fatalf("sync build command mismatch: %q", cfg.Sync.Build.Command)
	}
	if len(cfg.Sync.Roots.Assets) != 1 || cfg.Sync.Roots.Assets[0] != "public" {
		t.Fatalf("sync assets roots mismatch: %#v", cfg.Sync.Roots.Assets)
	}
	if len(cfg.Sync.Generated) != 1 || cfg.Sync.Generated[0] != "dist/**" {
		t.Fatalf("sync generated mismatch: %#v", cfg.Sync.Generated)
	}
}

func TestWarnUnknownSyncKeysYAML(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ProjectFileName)
	data := `sync:
  build:
    command: yarn
    argz:
      - build
  roots:
    assets:
      - images
    files:
      - bogus
  ignored: true
`

	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	warnings, err := WarnUnknownSyncKeys(path)
	if err != nil {
		t.Fatalf("warn unknown sync keys: %v", err)
	}

	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "sync") || !strings.Contains(joined, "ignored") {
		t.Fatalf("missing sync warning: %v", warnings)
	}
	if !strings.Contains(joined, "sync.build") || !strings.Contains(joined, "argz") {
		t.Fatalf("missing sync.build warning: %v", warnings)
	}
	if !strings.Contains(joined, "sync.roots") || !strings.Contains(joined, "files") {
		t.Fatalf("missing sync.roots warning: %v", warnings)
	}
}
