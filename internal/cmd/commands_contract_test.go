package cmd

import (
	"strings"
	"testing"
)

func TestCommandContractContainsCanonicalAPICommand(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	contract := buildCommandContract(parser.Model)
	for _, command := range contract.Commands {
		if command.Path == "nimbu api get" {
			if len(command.Arguments) != 1 || command.Arguments[0].Name != "path" {
				t.Fatalf("api get arguments = %#v", command.Arguments)
			}
			return
		}
	}
	t.Fatal("nimbu api get missing from command contract")
}

func TestCommandContractContainsAuditedNativeWorkflows(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}
	contract := buildCommandContract(parser.Model)
	paths := map[string]bool{}
	for _, command := range contract.Commands {
		paths[command.Path] = true
	}

	expected := []string{
		"nimbu apps code get", "nimbu apps code update", "nimbu apps code delete",
		"nimbu uploads download",
		"nimbu products attachments list", "nimbu products attachments download",
		"nimbu pages versions list", "nimbu pages versions restore",
		"nimbu customers roles list", "nimbu customers roles set",
		"nimbu roles customers add", "nimbu roles customers remove", "nimbu roles customers set",
		"nimbu announcements list", "nimbu announcements create",
		"nimbu domain-registrations list", "nimbu domain-registrations upsert",
		"nimbu settings get", "nimbu settings consent list",
		"nimbu events track", "nimbu events ingest",
	}
	for _, path := range expected {
		if !paths[path] {
			t.Errorf("command contract missing %q", path)
		}
	}
}

func TestCommandContractDoesNotExposeInternalAccountWorkflows(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	contract := buildCommandContract(parser.Model)
	for _, command := range contract.Commands {
		if strings.HasPrefix(command.Path, "nimbu accounts") {
			t.Fatalf("command contract exposes internal account workflow %q", command.Path)
		}
	}
}

func TestSiteFlagHelpMentionsProjectResolution(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}
	for _, flag := range buildCommandContract(parser.Model).GlobalFlags {
		if flag.Name != "site" {
			continue
		}
		help := strings.ToLower(flag.Help)
		for _, needle := range []string{"--site", "nimbu_site", "nimbu_project_dir", "git"} {
			if !strings.Contains(help, needle) {
				t.Fatalf("site flag help %q, missing %q", flag.Help, needle)
			}
		}
		return
	}
	t.Fatal("global --site flag missing from command contract")
}

func TestLocalizedListContractExplainsContentLocale(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}
	for _, command := range buildCommandContract(parser.Model).Commands {
		if command.Path != "nimbu pages list" {
			continue
		}
		for _, flag := range command.Flags {
			if flag.Name == "locale" {
				if !strings.Contains(strings.ToLower(flag.Help), "content locale") {
					t.Fatalf("pages list --locale help = %q, want content locale semantics", flag.Help)
				}
				return
			}
		}
		t.Fatal("pages list --locale missing")
	}
	t.Fatal("pages list missing from command contract")
}

func TestJobsRunContractDoesNotExposeAppFlag(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	contract := buildCommandContract(parser.Model)
	for _, command := range contract.Commands {
		if command.Path != "nimbu jobs run" {
			continue
		}
		for _, flag := range command.Flags {
			if flag.Name == "app" {
				t.Fatal("jobs run must resolve its owning app from the site-level job registry")
			}
		}
		return
	}
	t.Fatal("nimbu jobs run missing from command contract")
}
