package devproxy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestComputeTemplateFingerprintIncludesContent(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	file := filepath.Join(dir, "index.liquid")
	fixed := time.Unix(1_700_000_000, 0).UTC()

	if err := os.WriteFile(file, []byte("ab"), 0o644); err != nil {
		t.Fatalf("write first file: %v", err)
	}
	if err := os.Chtimes(file, fixed, fixed); err != nil {
		t.Fatalf("set first times: %v", err)
	}
	first, err := computeTemplateFingerprint(root)
	if err != nil {
		t.Fatalf("compute first fingerprint: %v", err)
	}

	if err := os.WriteFile(file, []byte("cd"), 0o644); err != nil {
		t.Fatalf("write second file: %v", err)
	}
	if err := os.Chtimes(file, fixed, fixed); err != nil {
		t.Fatalf("set second times: %v", err)
	}
	second, err := computeTemplateFingerprint(root)
	if err != nil {
		t.Fatalf("compute second fingerprint: %v", err)
	}

	if first == second {
		t.Fatalf("fingerprint should change when template content changes (first=%s second=%s)", first, second)
	}
}

func TestBuildCompressedTemplatesFailsOnUnreadableTemplate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 semantics differ on windows")
	}

	root := t.TempDir()
	dir := filepath.Join(root, "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	file := filepath.Join(dir, "private.liquid")
	if err := os.WriteFile(file, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.Chmod(file, 0o000); err != nil {
		t.Fatalf("chmod template: %v", err)
	}
	defer func() { _ = os.Chmod(file, 0o600) }()

	_, _, err := buildCompressedTemplates(root)
	if err == nil {
		t.Fatal("expected read error for unreadable template")
	}
}
