package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireWrite(t *testing.T) {
	if err := requireWrite(&RootFlags{Readonly: false}, "update resource"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := requireWrite(&RootFlags{Readonly: true}, "update resource")
	if err == nil {
		t.Fatal("expected readonly error")
	}
	if !strings.Contains(err.Error(), "readonly mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireForce(t *testing.T) {
	if err := requireForce(&RootFlags{Force: true}, "resource x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := requireForce(&RootFlags{Force: false}, "resource x")
	if err == nil {
		t.Fatal("expected force error")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadJSONInputFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "body.json")
	if err := os.WriteFile(file, []byte(`{"name":"nimbu","count":2}`), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	body, err := readJSONInput(file)
	if err != nil {
		t.Fatalf("readJSONInput: %v", err)
	}

	if body["name"] != "nimbu" {
		t.Fatalf("unexpected name: %v", body["name"])
	}
}

func TestReadJSONInputPreservesExactNumbers(t *testing.T) {
	file := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(file, []byte(`{"large":9007199254740993,"precise":0.12345678901234567890}`), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	body, err := readJSONInput(file)
	if err != nil {
		t.Fatalf("readJSONInput: %v", err)
	}
	if body["large"] != json.Number("9007199254740993") {
		t.Fatalf("large = %#v", body["large"])
	}
	if body["precise"] != json.Number("0.12345678901234567890") {
		t.Fatalf("precise = %#v", body["precise"])
	}
}

func TestReadJSONInputFromStdin(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r

	_, _ = w.WriteString(`{"ok":true}`)
	_ = w.Close()

	body, err := readJSONInput("-")
	if err != nil {
		t.Fatalf("readJSONInput: %v", err)
	}

	if body["ok"] != true {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestReadJSONInputRejectsTooLargePayload(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "too-large.json")
	data := strings.Repeat("a", int(maxJSONInputBytes)+1)
	if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := readJSONInput(file)
	if err == nil {
		t.Fatal("expected size error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadJSONInputRejectsEmptyInput(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	_ = w.Close()

	_, err = readJSONInput("-")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no JSON input") {
		t.Fatalf("unexpected error: %v", err)
	}
}
