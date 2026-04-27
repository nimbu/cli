package cmd

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

func TestChannelEntriesCopyDryRunDoesNotWrite(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "test-token")

	var writes int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels":
			_, _ = w.Write([]byte(`[{"id":"c1","slug":"articles","name":"Articles","customizations":[]}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/articles/entries":
			if r.Header.Get("X-Nimbu-Site") == "source" {
				_, _ = w.Write([]byte(`[{"id":"e1","slug":"hello-world","title":"Hello world"}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case (r.Method == http.MethodPost || r.Method == http.MethodPut) && strings.Contains(r.URL.Path, "/entries"):
			writes++
			http.Error(w, "unexpected write", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, &RootFlags{APIURL: srv.URL, Site: "source", Timeout: 2 * time.Second})
	cfg := config.Defaults()
	cfg.DefaultSite = "source"
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: discardWriter{}, Err: discardWriter{}, NoTTY: true})
	ctx = output.WithProgress(ctx, output.NewProgress(ctx))

	cmd := &ChannelEntriesCopyCmd{
		From:   "source/articles",
		To:     "target/articles",
		DryRun: true,
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "source", APIURL: srv.URL, Timeout: 2 * time.Second}); err != nil {
		t.Fatalf("run entries copy dry-run: %v", err)
	}
	if writes != 0 {
		t.Fatalf("expected no writes during dry-run, got %d", writes)
	}
}

func TestSitesCopyDryRunDoesNotWrite(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "test-token")

	var writes int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			writes++
			http.Error(w, "unexpected write", http.StatusInternalServerError)
			return
		}
		// Return empty lists for all GET endpoints so stages complete quickly.
		switch {
		case r.URL.Path == "/channels":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/uploads":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/roles":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/products":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/collections":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/themes":
			_, _ = w.Write([]byte(`[{"id":"t1","name":"default-theme","active":true}]`))
		case strings.HasPrefix(r.URL.Path, "/themes/"):
			_, _ = w.Write([]byte(`{"assets":[],"layouts":[],"snippets":[],"templates":[]}`))
		case r.URL.Path == "/pages":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/menus":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/blogs":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/notifications":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/redirects":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/translations":
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, &RootFlags{APIURL: srv.URL, Site: "source", Timeout: 2 * time.Second})
	cfg := config.Defaults()
	cfg.DefaultSite = "source"
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: discardWriter{}, Err: discardWriter{}, NoTTY: true})
	ctx = output.WithProgress(ctx, output.NewProgress(ctx))

	cmd := &SitesCopyCmd{
		From:   "source",
		To:     "target",
		DryRun: true,
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "source", APIURL: srv.URL, Timeout: 2 * time.Second}); err != nil {
		t.Fatalf("run sites copy dry-run: %v", err)
	}
	if writes != 0 {
		t.Fatalf("expected no writes during dry-run, got %d", writes)
	}
}

func TestReadSiteCopyConflictDecisionParsesDefaultAndBulkChoices(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		action   migrate.ExistingContentAction
		applyAll bool
	}{
		{name: "default update", input: "\n", action: migrate.ExistingContentUpdate},
		{name: "review items", input: "r\n", action: migrate.ExistingContentReview},
		{name: "skip this type", input: "n\n", action: migrate.ExistingContentSkip},
		{name: "update all remaining", input: "a\n", action: migrate.ExistingContentUpdate, applyAll: true},
		{name: "skip all remaining", input: "s\n", action: migrate.ExistingContentSkip, applyAll: true},
		{name: "abort", input: "q\n", action: migrate.ExistingContentAbort},
		{name: "help then update", input: "?\ny\n", action: migrate.ExistingContentUpdate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			got, err := readSiteCopyConflictDecision(
				bufio.NewReader(strings.NewReader(tt.input)),
				&out,
				migrate.ExistingContentPrompt{Type: "Menus", Source: "source", Target: "target"},
			)
			if err != nil {
				t.Fatalf("read decision: %v", err)
			}
			if got.Action != tt.action || got.ApplyToAll != tt.applyAll {
				t.Fatalf("decision = %#v, want action=%q applyAll=%v", got, tt.action, tt.applyAll)
			}
			if strings.Count(out.String(), "already exist") > 1 {
				t.Fatalf("prompt repeated full summary after help:\n%s", out.String())
			}
		})
	}
}

func TestReadSiteCopyItemConflictDecisionParsesBulkChoices(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		action   migrate.ExistingContentAction
		applyAll bool
	}{
		{name: "default update item", input: "\n", action: migrate.ExistingContentUpdate},
		{name: "skip item", input: "n\n", action: migrate.ExistingContentSkip},
		{name: "update remaining items", input: "a\n", action: migrate.ExistingContentUpdate, applyAll: true},
		{name: "skip remaining items", input: "s\n", action: migrate.ExistingContentSkip, applyAll: true},
		{name: "abort", input: "q\n", action: migrate.ExistingContentAbort},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			got, err := readSiteCopyItemConflictDecision(
				bufio.NewReader(strings.NewReader(tt.input)),
				&out,
				migrate.ExistingContentPrompt{Type: "Channels", Item: "articles", Source: "source", Target: "target"},
			)
			if err != nil {
				t.Fatalf("read item decision: %v", err)
			}
			if got.Action != tt.action || got.ApplyToAll != tt.applyAll {
				t.Fatalf("decision = %#v, want action=%q applyAll=%v", got, tt.action, tt.applyAll)
			}
		})
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
