package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

// Regression: these list commands used to drop pagination metadata, so a
// truncated first page looked complete in every output mode.
func TestPagedListCommandsNoteMoreResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "50")
		_, _ = w.Write([]byte(`[{"id":"x1","key":"k1","name":"one","source":"/a","target":"/b"},{"id":"x2","key":"k2","name":"two","source":"/c","target":"/d"}]`))
	}))
	defer server.Close()

	commands := []struct {
		noun string
		run  func(context.Context) error
	}{
		{"themes", func(ctx context.Context) error { return (&ThemesListCmd{Page: 1, PerPage: 2}).Run(ctx, &RootFlags{}) }},
		{"snippets", func(ctx context.Context) error {
			return (&ThemeSnippetsListCmd{Theme: "t", Page: 1, PerPage: 2}).Run(ctx, &RootFlags{})
		}},
		{"templates", func(ctx context.Context) error {
			return (&ThemeTemplatesListCmd{Theme: "t", Page: 1, PerPage: 2}).Run(ctx, &RootFlags{})
		}},
		{"layouts", func(ctx context.Context) error {
			return (&ThemeLayoutsListCmd{Theme: "t", Page: 1, PerPage: 2}).Run(ctx, &RootFlags{})
		}},
		{"assets", func(ctx context.Context) error {
			return (&ThemeAssetsListCmd{Theme: "t", Page: 1, PerPage: 2}).Run(ctx, &RootFlags{})
		}},
		{"redirects", func(ctx context.Context) error {
			return (&RedirectsListCmd{Page: 1, PerPage: 2}).Run(ctx, &RootFlags{})
		}},
		{"roles", func(ctx context.Context) error { return (&RolesListCmd{Page: 1, PerPage: 2}).Run(ctx, &RootFlags{}) }},
		{"apps", func(ctx context.Context) error { return (&AppsListCmd{Page: 1, PerPage: 2}).Run(ctx, &RootFlags{}) }},
		{"announcements", func(ctx context.Context) error { return (&AnnouncementsListCmd{Page: 1, PerPage: 2}).Run(ctx) }},
		{"domain registrations", func(ctx context.Context) error {
			return (&DomainRegistrationsListCmd{Page: 1, PerPage: 2}).Run(ctx)
		}},
	}
	for _, tc := range commands {
		for _, mode := range []output.Mode{{JSON: true}, {Plain: true}, {}} {
			ctx, stdout, stderr := newContractTestContext(t, server.URL, mode)
			if err := tc.run(ctx); err != nil {
				t.Fatalf("%s %+v: %v", tc.noun, mode, err)
			}
			want := "nimbu: showing 2 of 50 " + tc.noun + "; pass --all or --page 2"
			if mode == (output.Mode{}) {
				want = "Showing 2 of 50 " + tc.noun + " (page 1, per-page 2)."
			}
			if !strings.Contains(stderr.String(), want) {
				t.Fatalf("%s %+v: stderr = %q, want %q", tc.noun, mode, stderr.String(), want)
			}
			if strings.Contains(stdout.String(), "of 50") {
				t.Fatalf("%s %+v: notice leaked to stdout: %q", tc.noun, mode, stdout.String())
			}
		}
	}
}

func TestThemeFilesListNotesMoreResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		name := strings.TrimPrefix(r.URL.Path, "/themes/t/")
		_, _ = w.Write([]byte(`[{"id":"` + name + `1","name":"` + name + `-a.liquid"},{"id":"` + name + `2","name":"` + name + `-b.liquid"}]`))
	}))
	defer server.Close()

	ctx, _, stderr := newContractTestContext(t, server.URL, output.Mode{Plain: true})
	if err := (&ThemeFilesListCmd{Theme: "t", Page: 1, PerPage: 1}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "nimbu: showing 1 of ") || !strings.Contains(stderr.String(), "files; pass --all or --page 2") {
		t.Fatalf("stderr = %q, want a more-results notice", stderr.String())
	}
}
