package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

func TestConfirmThemeOverwriteReusesReaderAcrossPrompts(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "answers")
	if err != nil {
		t.Fatalf("create input: %v", err)
	}
	if _, err := input.WriteString("n\ny\n"); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("rewind input: %v", err)
	}

	originalStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = originalStdin
		_ = input.Close()
	})

	var stderr bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{
		Out:   &bytes.Buffer{},
		Err:   &stderr,
		Color: "never",
		NoTTY: true,
	})
	confirm := confirmThemeOverwrite(&RootFlags{})
	conflict := &api.Error{StatusCode: 409, Message: "Conflict (Peter edited article.liquid)"}

	first, err := confirm(ctx, themes.Resource{DisplayPath: "templates/article.liquid"}, conflict)
	if err != nil {
		t.Fatalf("first prompt: %v", err)
	}
	second, err := confirm(ctx, themes.Resource{DisplayPath: "templates/page.liquid"}, conflict)
	if err != nil {
		t.Fatalf("second prompt: %v", err)
	}

	if first || !second {
		t.Fatalf("unexpected answers: first=%v second=%v", first, second)
	}
	if got := stderr.String(); !strings.Contains(got, "skipping upload of templates/article.liquid") || !strings.Contains(got, "forcing upload of templates/page.liquid") {
		t.Fatalf("unexpected stderr: %q", got)
	}
}

func TestThemeTransferTimelineLabelUsesThemeSlugAndSiteSubdomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/themes/69b27878d0e7a2292b1bba9a":
			_, _ = w.Write([]byte(`{"theme":{"id":"69b27878d0e7a2292b1bba9a","name":"Theme Zenjoy 2026","short":"theme-zenjoy-2026"}}`))
		case "/sites/gegl96z":
			_, _ = w.Write([]byte(`{"id":"site-1","subdomain":"zenjoy"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := api.New(server.URL, "")
	got := themeTransferTimelineLabel(context.Background(), client, "69b27878d0e7a2292b1bba9a", "gegl96z")

	if got != "theme-zenjoy-2026 (zenjoy)" {
		t.Fatalf("unexpected label: %q", got)
	}
}

func TestThemeTransferTimelineLabelFallsBackWhenThemeInfoUnavailable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client := api.New(server.URL, "")
	got := themeTransferTimelineLabel(context.Background(), client, "69b27878d0e7a2292b1bba9a", "site-1")

	if got != "69b27878d0e7a2292b1bba9a (site-1)" {
		t.Fatalf("unexpected label: %q", got)
	}
}

func TestThemePushAcceptsOnlySelectors(t *testing.T) {
	parser, cli, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	if _, err := parser.Parse([]string{
		"themes",
		"push",
		"--only=javascript/*.js",
		"--only=stylesheets/*.css",
		"--only=layouts/default.liquid",
		"--only=snippets/bundle_app.liquid",
	}); err != nil {
		t.Fatalf("parse themes push selectors: %v", err)
	}

	want := []string{
		"javascript/*.js",
		"stylesheets/*.css",
		"layouts/default.liquid",
		"snippets/bundle_app.liquid",
	}
	if got := cli.Themes.Push.Only; !reflect.DeepEqual(got, want) {
		t.Fatalf("selectors = %#v, want %#v", got, want)
	}

	opts := themePushOptions(&cli.Themes.Push, nil)
	if got := opts.Only; !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded selectors = %#v, want %#v", got, want)
	}
}

func TestThemeSyncAcceptsOnlySelectors(t *testing.T) {
	parser, cli, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	if _, err := parser.Parse([]string{
		"themes",
		"sync",
		"--only=stylesheets/*.css",
		"--only=snippets/bundle_app.liquid",
	}); err != nil {
		t.Fatalf("parse themes sync selectors: %v", err)
	}

	want := []string{"stylesheets/*.css", "snippets/bundle_app.liquid"}
	if got := cli.Themes.Sync.Only; !reflect.DeepEqual(got, want) {
		t.Fatalf("selectors = %#v, want %#v", got, want)
	}

	opts := themeSyncOptions(&cli.Themes.Sync, nil)
	if got := opts.Only; !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded selectors = %#v, want %#v", got, want)
	}
}

func TestThemePushAndSyncExpandCommaSeparatedOnlySelectors(t *testing.T) {
	push := themePushOptions(&ThemePushCmd{
		Only: []string{"javascript/*.js, stylesheets/*.css", "layouts/default.liquid"},
	}, nil)
	wantPush := []string{"javascript/*.js", "stylesheets/*.css", "layouts/default.liquid"}
	if !reflect.DeepEqual(push.Only, wantPush) {
		t.Fatalf("push selectors = %#v, want %#v", push.Only, wantPush)
	}

	sync := themeSyncOptions(&ThemeSyncCmd{
		Only: []string{"stylesheets/*.css,snippets/bundle_app.liquid"},
	}, nil)
	wantSync := []string{"stylesheets/*.css", "snippets/bundle_app.liquid"}
	if !reflect.DeepEqual(sync.Only, wantSync) {
		t.Fatalf("sync selectors = %#v, want %#v", sync.Only, wantSync)
	}
}
