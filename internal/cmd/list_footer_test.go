package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func footerContext(mode output.Mode) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: stdout, Err: stderr, Mode: mode})
	return output.WithMode(ctx, mode), stdout, stderr
}

func TestWriteListFooter(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		meta     listFooterMeta
		want     string
	}{
		{
			name:     "known total with more pages",
			resource: "sites",
			meta:     listFooterMeta{Page: 1, PerPage: 3, Returned: 3, Total: 327, TotalKnown: true},
			want:     "\nShowing 3 of 327 sites (page 1, per-page 3). Use --page 2 for more, or --all for everything.\n",
		},
		{
			name:     "last page of a known total",
			resource: "sites",
			meta:     listFooterMeta{Page: 3, PerPage: 3, Returned: 1, Total: 7, TotalKnown: true},
			want:     "\nShowing 1 of 7 sites (page 3, per-page 3). Use --all for full results.\n",
		},
		{
			name:     "unknown total with next page",
			resource: "entries",
			meta:     listFooterMeta{Page: 2, PerPage: 1, Returned: 1, HasNext: true},
			want:     "\nShowing 1 entry on page 2; more results available. Use --page 3 for more, or --all for everything.\n",
		},
		{
			name:     "everything fit, total unknown",
			resource: "channels",
			meta:     listFooterMeta{Page: 1, PerPage: 25, Returned: 1},
		},
		{
			name:     "everything fit, total known",
			resource: "channels",
			meta:     listFooterMeta{Page: 1, PerPage: 25, Returned: 4, Total: 4, TotalKnown: true},
		},
		{
			name:     "--all",
			resource: "sites",
			meta:     allListFooterMeta(12),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, stdout, stderr := footerContext(output.Mode{})
			if err := writeListFooter(ctx, tc.resource, tc.meta); err != nil {
				t.Fatal(err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("footer must not go to stdout, got %q", stdout.String())
			}
			if got := stderr.String(); got != tc.want {
				t.Fatalf("stderr = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWriteListFooterSkipsMachineModes(t *testing.T) {
	for _, mode := range []output.Mode{{JSON: true}, {Plain: true}} {
		ctx, stdout, stderr := footerContext(mode)
		meta := listFooterMeta{Page: 1, PerPage: 25, Returned: 25, Total: 100, TotalKnown: true}
		if err := writeListFooter(ctx, "products", meta); err != nil {
			t.Fatal(err)
		}
		if stdout.Len()+stderr.Len() != 0 {
			t.Fatalf("expected no human footer in %+v, got stdout=%q stderr=%q", mode, stdout.String(), stderr.String())
		}
	}
}

func TestNoteMoreResults(t *testing.T) {
	tests := []struct {
		name string
		mode output.Mode
		meta listFooterMeta
		want string
	}{
		{
			name: "json known total",
			mode: output.Mode{JSON: true},
			meta: listFooterMeta{Page: 1, PerPage: 25, Returned: 25, Total: 327, TotalKnown: true},
			want: "nimbu: showing 25 of 327 products; pass --all or --page 2\n",
		},
		{
			name: "plain unknown total",
			mode: output.Mode{Plain: true},
			meta: listFooterMeta{Page: 4, PerPage: 1, Returned: 1, HasNext: true},
			want: "nimbu: showing 1 product; pass --all or --page 5\n",
		},
		{
			name: "json complete listing",
			mode: output.Mode{JSON: true},
			meta: listFooterMeta{Page: 1, PerPage: 25, Returned: 3, Total: 3, TotalKnown: true},
		},
		{
			name: "human mode is left to the footer",
			mode: output.Mode{},
			meta: listFooterMeta{Page: 1, PerPage: 25, Returned: 25, Total: 327, TotalKnown: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, stdout, stderr := footerContext(tc.mode)
			noteMoreResults(ctx, "products", tc.meta)
			if stdout.Len() != 0 {
				t.Fatalf("notice must not go to stdout, got %q", stdout.String())
			}
			if got := stderr.String(); got != tc.want {
				t.Fatalf("stderr = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPluralNoun(t *testing.T) {
	cases := map[string]string{
		"channels":         "channel",
		"entries":          "entry",
		"translation keys": "translation key",
		"articles":         "article",
	}
	for plural, singular := range cases {
		if got := pluralNoun(1, plural); got != singular {
			t.Fatalf("pluralNoun(1, %q) = %q, want %q", plural, got, singular)
		}
		if got := pluralNoun(2, plural); got != plural {
			t.Fatalf("pluralNoun(2, %q) = %q", plural, got)
		}
	}
}

func TestSitesListKeepsStdoutToRowsAndNotesMorePagesOnStderr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/sites/count" {
			_, _ = w.Write([]byte(`{"count":327}`))
			return
		}
		w.Header().Set("X-Total-Count", "327")
		_, _ = w.Write([]byte(`[{"id":"s1","subdomain":"a","name":"A"},{"id":"s2","subdomain":"b","name":"B"},{"id":"s3","subdomain":"c","name":"C"}]`))
	}))
	defer server.Close()

	for _, tc := range []struct {
		mode       output.Mode
		stdoutRows int
		wantStderr string
	}{
		{mode: output.Mode{}, stdoutRows: 4, wantStderr: "Showing 3 of 327 sites (page 1, per-page 3)."},
		{mode: output.Mode{Plain: true}, stdoutRows: 3, wantStderr: "nimbu: showing 3 of 327 sites; pass --all or --page 2"},
		{mode: output.Mode{JSON: true}, wantStderr: "nimbu: showing 3 of 327 sites; pass --all or --page 2"},
	} {
		ctx, stdout, stderr := newSitesListTestContext(t, server.URL, tc.mode)
		cmd := SitesListCmd{Page: 1, PerPage: 3}
		if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if tc.mode.JSON {
			var rows []map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil || len(rows) != 3 {
				t.Fatalf("expected a bare 3-element JSON array on stdout, got %q (%v)", stdout.String(), err)
			}
		} else if got := strings.Count(stdout.String(), "\n"); got != tc.stdoutRows {
			t.Fatalf("mode %+v: stdout has %d lines, want %d:\n%s", tc.mode, got, tc.stdoutRows, stdout.String())
		}
		if strings.Contains(stdout.String(), "327") {
			t.Fatalf("mode %+v: pagination notice leaked to stdout:\n%s", tc.mode, stdout.String())
		}
		if !strings.Contains(stderr.String(), tc.wantStderr) {
			t.Fatalf("mode %+v: stderr = %q, want %q", tc.mode, stderr.String(), tc.wantStderr)
		}
	}
}
