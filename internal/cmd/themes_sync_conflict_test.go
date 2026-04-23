package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestThemeTransferTimelineLabelUsesThemeInfoShortIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/themes/69b27878d0e7a2292b1bba9a/info" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"theme_short_id":"storefront","site_short_id":"acme"}`))
	}))
	defer server.Close()

	client := api.New(server.URL, "")
	got := themeTransferTimelineLabel(context.Background(), client, "69b27878d0e7a2292b1bba9a", "site-1")

	if got != "storefront (acme)" {
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
