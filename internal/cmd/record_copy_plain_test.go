package cmd

import (
	"bytes"
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

func plainCopyContext(t *testing.T, apiURL string, out *bytes.Buffer) context.Context {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")
	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, &RootFlags{APIURL: apiURL, Site: "source", Timeout: 2 * time.Second})
	cfg := config.Defaults()
	cfg.DefaultSite = "source"
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, output.Mode{Plain: true})
	ctx = output.WithWriter(ctx, &output.Writer{Out: out, Err: discardWriter{}, NoTTY: true})
	return output.WithProgress(ctx, output.NewProgress(ctx))
}

func TestWriteRecordCopyPlainEscapesFields(t *testing.T) {
	var out bytes.Buffer
	ctx := output.WithMode(context.Background(), output.Mode{Plain: true})
	ctx = output.WithWriter(ctx, &output.Writer{Out: &out, Err: discardWriter{}, NoTTY: true})

	err := writeRecordCopyPlain(ctx, []migrate.RecordCopyItem{
		{Action: "create", Resource: "articles", Identifier: "a\tb\nc\rd\\e", TargetID: "t1"},
		{Action: "update", Resource: "articles", Identifier: "plain", TargetID: ""},
	})
	if err != nil {
		t.Fatalf("writeRecordCopyPlain: %v", err)
	}
	want := "create\tarticles\ta\\tb\\nc\\rd\\\\e\tt1\n" +
		"update\tarticles\tplain\t\n"
	if got := out.String(); got != want {
		t.Fatalf("plain rows:\n got %q\nwant %q", got, want)
	}
}

func TestChannelEntriesCopyPlainEscapesIdentifier(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels":
			_, _ = w.Write([]byte(`[{"id":"c1","slug":"articles","name":"Articles","customizations":[]}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/articles/entries":
			if r.Header.Get("X-Nimbu-Site") == "source" {
				_, _ = w.Write([]byte(`[{"id":"e1","slug":"hello\tworld\nagain"}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		default:
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	ctx := plainCopyContext(t, srv.URL, &out)
	cmd := &ChannelEntriesCopyCmd{From: "source/articles", To: "target/articles", DryRun: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "source", APIURL: srv.URL, Timeout: 2 * time.Second}); err != nil {
		t.Fatalf("run entries copy: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one plain row, got %q", out.String())
	}
	fields := strings.Split(lines[0], "\t")
	if len(fields) != 4 || fields[2] != `hello\tworld\nagain` {
		t.Fatalf("expected 4 fields with escaped identifier, got %q", fields)
	}
}

func TestCustomersCopyPlainEscapesIdentifier(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		source := r.Header.Get("X-Nimbu-Site") == "source"
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/customers/customizations":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/customers" && source:
			_, _ = w.Write([]byte(`[{"id":"c1","email":"a\tb\\c@example.test"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/customers":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/customers" && !source:
			_, _ = w.Write([]byte(`{"id":"t1"}`))
		default:
			t.Logf("unexpected request %s %s (site %s)", r.Method, r.URL.String(), r.Header.Get("X-Nimbu-Site"))
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	ctx := plainCopyContext(t, srv.URL, &out)
	cmd := &CustomersCopyCmd{From: "source", To: "target"}
	if err := cmd.Run(ctx, &RootFlags{Site: "source", APIURL: srv.URL, Timeout: 2 * time.Second}); err != nil {
		t.Fatalf("run customers copy: %v", err)
	}
	want := "create\tcustomers\ta\\tb\\\\c@example.test\tt1\n"
	if got := out.String(); got != want {
		t.Fatalf("plain rows:\n got %q\nwant %q", got, want)
	}
}
