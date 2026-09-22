package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateEnvironmentPreservesProjectYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nimbu.yml")
	original := "# project comment\nsite: original\nfuture:\n  enabled: true\nenvironments:\n  staging: {host: api.example.test, site: staging-site, extra: retained}\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	env := EnvironmentConfig{Host: "api.nimbu.io", Site: "prod", Protected: true}
	if err := UpdateEnvironment(path, "production", &env); err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadProjectConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environments["production"] != env {
		t.Fatalf("environment = %#v", cfg.Environments["production"])
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"# project comment", "future:", "extra: retained"} {
		if !strings.Contains(string(data), expected) {
			t.Errorf("missing %q in %s", expected, data)
		}
	}
	if err := UpdateEnvironment(path, "production", &env); err == nil {
		t.Fatal("expected duplicate environment error")
	}
	if err := UpdateEnvironment(path, "production", nil); err != nil {
		t.Fatal(err)
	}
	cfg, err = ReadProjectConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Environments["production"]; ok {
		t.Fatal("removed environment remains")
	}
	if len(cfg.Environments) != 1 {
		t.Fatalf("environments = %#v", cfg.Environments)
	}
}

func TestWarnUnknownEnvironmentKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nimbu.yml")
	if err := os.WriteFile(path, []byte("environments:\n  prod: {host: api.nimbu.io, site: prod, protected: true, protcted: true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	warnings, err := WarnUnknownEnvironmentKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0] != "unknown environments.prod key: protcted" {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestUpdateEnvironmentRejectsMultipleDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nimbu.yml")
	original := "site: original\n---\nfuture: retained\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	err := UpdateEnvironment(path, "prod", &EnvironmentConfig{Host: "api.test", Site: "prod"})
	if err == nil || !strings.Contains(err.Error(), "single YAML document") {
		t.Fatalf("error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != original {
		t.Fatalf("project changed: %s; %v", data, err)
	}
}

func TestUpdateEnvironmentRejectsWhitespaceNames(t *testing.T) {
	for _, name := range []string{"prod\rstaging", "prod\u00a0staging", "prod\vstaging", "prod\\staging"} {
		err := UpdateEnvironment("unused", name, &EnvironmentConfig{Host: "api.test", Site: "prod"})
		if err == nil || !strings.Contains(err.Error(), "environment name") {
			t.Errorf("name %q: %v", name, err)
		}
	}
}
