package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestSitesListRunJSONFieldsOnlyOutputsRequestedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sites" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("fields"); got != "id,subdomain,name" {
			t.Fatalf("fields query = %q, want id,subdomain,name", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"site-1","subdomain":"demo","name":"Demo Site"}]`))
	}))
	defer server.Close()

	ctx, stdout, _ := newSitesListTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := SitesListCmd{
		QueryFlags: QueryFlags{Fields: "id,subdomain,name"},
		All:        true,
	}

	if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL}); err != nil {
		t.Fatalf("run: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, stdout.String())
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}

	wantKeys := []string{"id", "subdomain", "name"}
	for _, key := range wantKeys {
		if _, ok := got[0][key]; !ok {
			t.Fatalf("missing requested field %q in %#v", key, got[0])
		}
	}
	for key := range got[0] {
		if !strings.Contains(","+strings.Join(wantKeys, ",")+",", ","+key+",") {
			t.Fatalf("unexpected field %q in %#v", key, got[0])
		}
	}
}

func newSitesListTestContext(t *testing.T, apiURL string, mode output.Mode) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	ctx = output.WithMode(ctx, mode)
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   &stdout,
		Err:   &stderr,
		Mode:  mode,
		Color: "never",
		NoTTY: true,
	})
	ctx = context.WithValue(ctx, rootFlagsKey{}, &RootFlags{
		APIURL:  apiURL,
		Timeout: 2 * time.Second,
	})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})
	return ctx, &stdout, &stderr
}
