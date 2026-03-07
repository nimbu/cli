package notifications

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestWriteNotificationsWritesLegacyDiskContract(t *testing.T) {
	root := t.TempDir()
	written, err := WriteNotifications(root, []api.Notification{{
		Slug:        "order_created",
		Name:        "Order created",
		Description: "desc",
		Subject:     "Hello",
		Text:        "Body",
		HTML:        "<p>Body</p>",
		HTMLEnabled: true,
		Translations: map[string]map[string]any{
			"nl": {"subject": "Hallo", "text": "Tekst", "html": "<p>Tekst</p>"},
			"fr": {"subject": "Hello", "text": "Body", "html": "<p>Body</p>"},
		},
	}}, nil)
	if err != nil {
		t.Fatalf("write notifications: %v", err)
	}

	if len(written) != 4 {
		t.Fatalf("written count = %d", len(written))
	}
	data, err := os.ReadFile(filepath.Join(root, "order_created.txt"))
	if err != nil {
		t.Fatalf("read base txt: %v", err)
	}
	text := string(data)
	for _, needle := range []string{"description: desc", "name: Order created", "subject: Hello", "Body"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in %q", needle, text)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "fr", "order_created.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected fr translation omitted, err=%v", err)
	}
}

func TestBuildNotificationsReadsDiskContract(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "order_created.txt"), []byte("---\nname: Order created\ndescription: desc\nsubject: Hello\n---\n\nBody"), 0o644); err != nil {
		t.Fatalf("write base txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "order_created.html"), []byte("<p>Body</p>"), 0o644); err != nil {
		t.Fatalf("write base html: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "nl"), 0o755); err != nil {
		t.Fatalf("mkdir locale: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "nl", "order_created.txt"), []byte("---\nsubject: Hallo\n---\n\nTekst"), 0o644); err != nil {
		t.Fatalf("write locale txt: %v", err)
	}

	notifications, err := BuildNotifications(root, nil, AllowedLocales([]string{"nl"}))
	if err != nil {
		t.Fatalf("build notifications: %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("notification count = %d", len(notifications))
	}
	got := notifications[0]
	if !got.HTMLEnabled || got.HTML != "<p>Body</p>" {
		t.Fatalf("html mismatch: %#v", got)
	}
	if got.Text != "Body" {
		t.Fatalf("expected exact body, got %q", got.Text)
	}
	if got.Translations["nl"]["subject"] != "Hallo" {
		t.Fatalf("translation mismatch: %#v", got.Translations)
	}
}

func TestReadTemplatesRejectsMissingRequiredFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "broken.txt"), []byte("---\nname: Broken\n---\n\nBody"), 0o644); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	_, err := ReadTemplates(root, nil, AllowedLocales(nil))
	if err == nil || !strings.Contains(err.Error(), "description is missing") {
		t.Fatalf("expected description error, got %v", err)
	}
}

func TestReadTemplatesRejectsUnsupportedLocaleDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "order_created.txt"), []byte("---\nname: Order created\ndescription: desc\nsubject: Hello\n---\n\nBody"), 0o644); err != nil {
		t.Fatalf("write base txt: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pirate"), 0o755); err != nil {
		t.Fatalf("mkdir locale: %v", err)
	}

	_, err := ReadTemplates(root, nil, AllowedLocales([]string{"nl"}))
	if err == nil || !strings.Contains(err.Error(), `unsupported locale directory "pirate"`) {
		t.Fatalf("expected unsupported locale error, got %v", err)
	}
}
