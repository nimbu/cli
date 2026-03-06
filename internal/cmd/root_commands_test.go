package cmd

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCLIHasNewTopLevelCommands(t *testing.T) {
	rt := reflect.TypeOf(CLI{})
	required := []string{
		"Accounts",
		"Collections",
		"Coupons",
		"Notifications",
		"Roles",
		"Redirects",
		"Functions",
		"Jobs",
		"Apps",
		"Server",
	}

	for _, field := range required {
		if _, ok := rt.FieldByName(field); !ok {
			t.Fatalf("CLI missing %s command", field)
		}
	}
}

func TestReadmeMentionsNewTopLevelCommands(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	readme := string(data)
	required := []string{
		"nimbu-cli accounts",
		"nimbu-cli collections",
		"nimbu-cli coupons",
		"nimbu-cli notifications",
		"nimbu-cli roles",
		"nimbu-cli redirects",
		"nimbu-cli functions",
		"nimbu-cli jobs",
		"nimbu-cli apps",
		"nimbu-cli server",
		"nimbu-cli themes push",
		"nimbu-cli themes sync",
	}

	for _, needle := range required {
		if !strings.Contains(readme, needle) {
			t.Fatalf("README missing command entry: %s", needle)
		}
	}
}

func TestReadmeDocumentsInlinePayloadSyntax(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	readme := string(data)
	required := []string{
		"## Inline Payload Syntax",
		"key=value",
		"key:=json",
		"key=@file.txt",
		"key:=@file.json",
		"translations update activate.label.lastname nl=Achternaam",
	}

	for _, needle := range required {
		if !strings.Contains(readme, needle) {
			t.Fatalf("README missing inline payload docs: %s", needle)
		}
	}
}

func TestAppendRootInlinePayloadFooter(t *testing.T) {
	input := "Usage: nimbu-cli <command> [flags]\n\nCLI for the Nimbu API\n\nCommands:\n  sites\n"
	out := appendRootInlinePayloadFooter(input)

	required := []string{
		"Create/Update supports inline payloads using:",
		"key=value",
		"key:=json",
		"key=@file.txt",
		"key:=@file.json",
	}
	for _, needle := range required {
		if !strings.Contains(out, needle) {
			t.Fatalf("footer missing %q", needle)
		}
	}

	if !strings.HasSuffix(out, "Create/Update supports inline payloads using: key=value, key:=json, key=@file.txt or key:=@file.json\n") {
		t.Fatalf("footer should be at bottom, got: %q", out)
	}

	out2 := appendRootInlinePayloadFooter(out)
	if out2 != out {
		t.Fatal("footer should not be appended twice")
	}
}

func TestCompactCommandsSection(t *testing.T) {
	input := "Usage: nimbu-cli <command> [flags]\n\nCommands:\n  auth <command> [flags]\n    Authentication and credentials\n\n  sites <command> [flags]\n    Manage sites\n\nRun \"nimbu-cli <command> --help\" for more information on a command.\n"
	out := compactCommandsSection(input)

	if strings.Contains(out, "\n    Authentication and credentials") {
		t.Fatalf("description should be collapsed to single line, got: %q", out)
	}
	if !strings.Contains(out, "auth <command> [flags]") || !strings.Contains(out, "· Authentication and credentials") {
		t.Fatalf("auth row not compacted: %q", out)
	}
	if !strings.Contains(out, "sites <command> [flags]") || !strings.Contains(out, "· Manage sites") {
		t.Fatalf("sites row not compacted: %q", out)
	}
}
