package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestNotificationsPullWritesLegacyFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/notifications":
			_, _ = w.Write([]byte(`[{"slug":"welcome","name":"Welcome","description":"Welcome mail","subject":"Hi","text":"Body","html_enabled":true,"html":"<p>Body</p>","translations":{"nl":{"subject":"Hoi","text":"Tekst","html":"<p>Tekst</p>"}}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newNotificationSyncTestContext(t, srv.URL)
	withTempCWD(t, t.TempDir(), func() {
		cmd := &NotificationsPullCmd{}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run pull: %v", err)
		}
		base := filepath.Join("content", "notifications", "welcome.txt")
		data, err := os.ReadFile(base)
		if err != nil {
			t.Fatalf("read %s: %v", base, err)
		}
		if !strings.Contains(string(data), "description: Welcome mail") {
			t.Fatalf("unexpected base template: %s", string(data))
		}
		if _, err := os.Stat(filepath.Join("content", "notifications", "nl", "welcome.html")); err != nil {
			t.Fatalf("expected locale html file: %v", err)
		}
	})
}

func TestNotificationsPushUpsertsExistingNotification(t *testing.T) {
	var gotMethod string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sites/demo":
			_, _ = w.Write([]byte(`{"id":"demo","locales":["nl"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/notifications/welcome":
			_, _ = w.Write([]byte(`{"slug":"welcome"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/notifications/welcome":
			gotMethod = r.Method
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode put body: %v", err)
			}
			_, _ = w.Write([]byte(`{"slug":"welcome"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newNotificationSyncTestContext(t, srv.URL)
	withTempCWD(t, t.TempDir(), func() {
		root := filepath.Join("content", "notifications")
		if err := os.MkdirAll(filepath.Join(root, "nl"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "welcome.txt"), []byte("---\ndescription: Welcome mail\nname: Welcome\nsubject: Hi\n---\n\nBody"), 0o644); err != nil {
			t.Fatalf("write base: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "nl", "welcome.txt"), []byte("---\nsubject: Hoi\n---\n\nTekst"), 0o644); err != nil {
			t.Fatalf("write locale: %v", err)
		}
		cmd := &NotificationsPushCmd{}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run push: %v", err)
		}
		if gotMethod != http.MethodPut {
			t.Fatalf("expected PUT, got %s", gotMethod)
		}
		translations := gotBody["translations"].(map[string]any)
		nl := translations["nl"].(map[string]any)
		if nl["subject"] != "Hoi" || nl["text"] != "Tekst" {
			t.Fatalf("unexpected translation body: %#v", nl)
		}
	})
}

func newNotificationSyncTestContext(t *testing.T, apiURL string) (context.Context, *strings.Builder, *strings.Builder) {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")

	flags := &RootFlags{APIURL: apiURL, Site: "demo"}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"

	out := &strings.Builder{}
	errOut := &strings.Builder{}

	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   out,
		Err:   errOut,
		Mode:  output.Mode{},
		NoTTY: true,
	})
	return ctx, out, errOut
}

func withTempCWD(t *testing.T, dir string, fn func()) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	defer func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()
	fn()
}
