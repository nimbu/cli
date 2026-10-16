package cmd

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/nimbu/cli/internal/output"
)

func manyScopes(n int) []string {
	scopes := make([]string, n)
	for i := range scopes {
		scopes[i] = fmt.Sprintf("read_scope_%02d", i)
	}
	return scopes
}

func TestScopeLinesShowsShortListInline(t *testing.T) {
	got := scopeLines([]string{"read_channels", "write_channels"}, false, 80)
	if !slices.Equal(got, []string{"  Scopes: read_channels, write_channels"}) {
		t.Fatalf("lines = %q", got)
	}
	if got := scopeLines(nil, true, 80); got != nil {
		t.Fatalf("no scopes = %q, want nothing", got)
	}
}

// Regression: 22 scopes were printed on one line that ran far past the
// terminal edge.
func TestScopeLinesSummarizesLongList(t *testing.T) {
	got := scopeLines(manyScopes(22), false, 80)
	if !slices.Equal(got, []string{"  Scopes: 22 granted (--verbose lists them)"}) {
		t.Fatalf("lines = %q", got)
	}
}

func TestScopeLinesWrapsWhenVerbose(t *testing.T) {
	scopes := manyScopes(22)
	for _, width := range []int{40, 80, 120} {
		lines := scopeLines(scopes, true, width)
		if len(lines) < 2 {
			t.Fatalf("width %d: expected wrapping, got %q", width, lines)
		}
		var seen []string
		for i, line := range lines {
			if w := ansi.StringWidth(line); w > width {
				t.Fatalf("width %d: line %d is %d cells: %q", width, i, w, line)
			}
			prefix := "          "
			if i == 0 {
				prefix = "  Scopes: "
			}
			if !strings.HasPrefix(line, prefix) {
				t.Fatalf("width %d: line %d = %q, want prefix %q", width, i, line, prefix)
			}
			for _, field := range strings.Fields(strings.TrimPrefix(line, prefix)) {
				seen = append(seen, strings.TrimSuffix(field, ","))
			}
		}
		if !slices.Equal(seen, scopes) {
			t.Fatalf("width %d: scopes lost or reordered: %q", width, seen)
		}
	}
}

func TestStatusStylerFollowsStdoutProfile(t *testing.T) {
	off := statusStyler{}
	if got := off.symbol("✓", "#22c55e"); got != "✓" {
		t.Fatalf("color off: symbol = %q", got)
	}
	if got := off.bold("host"); got != "host" {
		t.Fatalf("color off: bold = %q", got)
	}

	truecolor := statusStyler{on: true, profile: colorprofile.TrueColor}.symbol("✓", "#22c55e")
	if !strings.Contains(truecolor, "38;2;34;197;94") {
		t.Fatalf("truecolor symbol = %q", truecolor)
	}
	ansi256 := statusStyler{on: true, profile: colorprofile.ANSI256}.symbol("✓", "#22c55e")
	if strings.Contains(ansi256, "38;2;") || !strings.Contains(ansi256, "38;5;") {
		t.Fatalf("256-color symbol = %q, want an indexed color", ansi256)
	}
	if ansi.Strip(ansi256) != "✓" {
		t.Fatalf("styled symbol text = %q", ansi.Strip(ansi256))
	}
}

func TestNewStatusStylerUsesStdoutDecision(t *testing.T) {
	st := newStatusStyler(&output.Writer{Out: &strings.Builder{}, Color: "never"})
	if st.on {
		t.Fatal("--color=never must turn status color off")
	}
	t.Setenv("NO_COLOR", "")
	st = newStatusStyler(&output.Writer{Out: &strings.Builder{}, Color: "always"})
	if !st.on || st.profile == colorprofile.ASCII {
		t.Fatalf("--color=always: on=%v profile=%v", st.on, st.profile)
	}
}

func TestAuthStatusSummarizesScopesAndKeepsJSONList(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "")
	fake := newOAuthAPI(t)
	cred := oauthSession("access-1", "refresh-1")
	cred.Scopes = manyScopes(22)
	withFakeAuthStore(t, &fakeAuthStore{credential: cred})

	ctx, stdout, _ := oauthTestContext(t, fake.URL, output.Mode{})
	if err := (&AuthStatusCmd{}).Run(ctx, ctx.Value(rootFlagsKey{}).(*RootFlags)); err != nil {
		t.Fatalf("auth status: %v", err)
	}
	if !strings.Contains(stdout.String(), "  Scopes: 22 granted (--verbose lists them)\n") {
		t.Fatalf("human output = %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("non-terminal stdout got escapes: %q", stdout.String())
	}

	ctx, stdout, _ = oauthTestContext(t, fake.URL, output.Mode{JSON: true})
	if err := (&AuthStatusCmd{}).Run(ctx, ctx.Value(rootFlagsKey{}).(*RootFlags)); err != nil {
		t.Fatalf("auth status --json: %v", err)
	}
	payload := decodeJSONOutput(t, stdout.Bytes())
	scopes, ok := payload["scopes"].([]any)
	if !ok || len(scopes) != 22 {
		t.Fatalf("json scopes = %v, want all 22", payload["scopes"])
	}
}
