package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// TestChannelEntriesListJSONEmitsEmptyArray guards against `null` output when a
// filtered list matches nothing; consumers piping to jq expect [].
func TestChannelEntriesListJSONEmitsEmptyArray(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "test-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/user":
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/entries/count"):
			_, _ = w.Write([]byte(`{"count":0}`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(srv.Close)

	flags := &RootFlags{APIURL: srv.URL, Site: "demo"}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"
	mode := output.Mode{JSON: true}

	out := &strings.Builder{}
	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, mode)
	ctx = output.WithWriter(ctx, &output.Writer{Out: out, Err: &strings.Builder{}, Mode: mode, NoTTY: true})

	cmd := &ChannelEntriesListCmd{Channel: "news", Page: 1, PerPage: 25}
	cmd.Filters = []string{"date>=2030-01-01"}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("list entries: %v", err)
	}

	if got := strings.TrimSpace(out.String()); got != "[]" {
		t.Fatalf("JSON output = %q, want []", got)
	}
}
