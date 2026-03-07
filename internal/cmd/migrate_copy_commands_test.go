package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/config"
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

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
