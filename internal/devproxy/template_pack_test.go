package devproxy

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	first, err := computeTemplateFingerprint(root, nil)
	if err != nil {
		t.Fatalf("compute first fingerprint: %v", err)
	}

	if err := os.WriteFile(file, []byte("cd"), 0o644); err != nil {
		t.Fatalf("write second file: %v", err)
	}
	if err := os.Chtimes(file, fixed, fixed); err != nil {
		t.Fatalf("set second times: %v", err)
	}
	second, err := computeTemplateFingerprint(root, nil)
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

	_, _, err := buildCompressedTemplates(root, nil)
	if err == nil {
		t.Fatal("expected read error for unreadable template")
	}
}

func TestBuildCompressedTemplatesIncludesVirtualOverlay(t *testing.T) {
	root := t.TempDir()

	compressed, _, err := buildCompressedTemplates(root, []TemplateOverlay{
		{Type: "snippets", Path: "bundle_app.liquid", Content: "virtual bundle"},
	})
	if err != nil {
		t.Fatalf("build compressed templates: %v", err)
	}

	templates := decodeCompressedTemplatesForTest(t, compressed)
	if got := templates["snippets"]["bundle_app.liquid"]; got != "virtual bundle" {
		t.Fatalf("overlay content mismatch: %q", got)
	}
}

func TestBuildCompressedTemplatesOverlayOverridesDiskTemplate(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "snippets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir snippets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bundle_app.liquid"), []byte("disk bundle"), 0o644); err != nil {
		t.Fatalf("write disk snippet: %v", err)
	}

	compressed, _, err := buildCompressedTemplates(root, []TemplateOverlay{
		{Type: "snippets", Path: "bundle_app.liquid", Content: "virtual bundle"},
	})
	if err != nil {
		t.Fatalf("build compressed templates: %v", err)
	}

	templates := decodeCompressedTemplatesForTest(t, compressed)
	if got := templates["snippets"]["bundle_app.liquid"]; got != "virtual bundle" {
		t.Fatalf("overlay should override disk template, got %q", got)
	}
}

func TestComputeTemplateFingerprintIncludesOverlayContent(t *testing.T) {
	root := t.TempDir()

	first, err := computeTemplateFingerprint(root, []TemplateOverlay{
		{Type: "snippets", Path: "bundle_app.liquid", Content: "first"},
	})
	if err != nil {
		t.Fatalf("compute first fingerprint: %v", err)
	}

	second, err := computeTemplateFingerprint(root, []TemplateOverlay{
		{Type: "snippets", Path: "bundle_app.liquid", Content: "second"},
	})
	if err != nil {
		t.Fatalf("compute second fingerprint: %v", err)
	}

	if first == second {
		t.Fatalf("fingerprint should change when overlay content changes")
	}
}

func TestNormalizedOverlaysRejectInvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		overlay  TemplateOverlay
		contains string
	}{
		{
			name:     "invalid type",
			overlay:  TemplateOverlay{Type: "assets", Path: "bundle_app.liquid", Content: "bad"},
			contains: "invalid overlay template type",
		},
		{
			name:     "traversal",
			overlay:  TemplateOverlay{Type: "snippets", Path: "../bundle_app.liquid", Content: "bad"},
			contains: "invalid overlay path",
		},
		{
			name:     "absolute",
			overlay:  TemplateOverlay{Type: "snippets", Path: "/tmp/bundle_app.liquid", Content: "bad"},
			contains: "invalid overlay path",
		},
		{
			name:     "wrong extension",
			overlay:  TemplateOverlay{Type: "snippets", Path: "bundle_app.js", Content: "bad"},
			contains: "must end in .liquid or .liquid.haml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := buildCompressedTemplates(t.TempDir(), []TemplateOverlay{tc.overlay})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tc.contains)
			}
		})
	}
}

func TestNormalizedOverlaysRejectDuplicateTypePath(t *testing.T) {
	_, _, err := buildCompressedTemplates(t.TempDir(), []TemplateOverlay{
		{Type: "snippets", Path: "bundle_app.liquid", Content: "first"},
		{Type: "snippets", Path: "./bundle_app.liquid", Content: "second"},
	})
	if err == nil {
		t.Fatal("expected duplicate overlay error")
	}
	if !strings.Contains(err.Error(), "duplicate overlay template") {
		t.Fatalf("error = %q, want duplicate overlay error", err.Error())
	}
}
