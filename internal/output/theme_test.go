package output

import (
	"strings"
	"testing"
	"time"

	"github.com/muesli/termenv"
)

func envMap(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestASCIIOnly(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "unset locale keeps unicode", env: map[string]string{}, want: false},
		{name: "utf-8 lang", env: map[string]string{"LANG": "en_US.UTF-8"}, want: false},
		{name: "utf8 spelling", env: map[string]string{"LANG": "C.utf8"}, want: false},
		{name: "C locale", env: map[string]string{"LANG": "C"}, want: true},
		{name: "latin1 ctype", env: map[string]string{"LC_CTYPE": "en_US.ISO8859-1", "LANG": "en_US.UTF-8"}, want: true},
		{name: "LC_ALL wins", env: map[string]string{"LC_ALL": "en_US.UTF-8", "LC_CTYPE": "C"}, want: false},
		{name: "dumb terminal", env: map[string]string{"TERM": "dumb", "LANG": "en_US.UTF-8"}, want: true},
		{name: "NIMBU_ASCII", env: map[string]string{"NIMBU_ASCII": "1", "LANG": "en_US.UTF-8"}, want: true},
		{name: "NIMBU_ASCII=0", env: map[string]string{"NIMBU_ASCII": "0"}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := asciiOnly(envMap(tc.env)); got != tc.want {
				t.Fatalf("asciiOnly() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnimationAllowed(t *testing.T) {
	if animationAllowed(false, true, envMap(map[string]string{"TERM": "dumb"})) {
		t.Fatal("TERM=dumb must not get cursor-moving animation")
	}
	if animationAllowed(true, true, envMap(nil)) {
		t.Fatal("--no-progress must disable animation")
	}
	if animationAllowed(false, false, envMap(nil)) {
		t.Fatal("non-terminal stderr must not animate")
	}
	if !animationAllowed(false, true, envMap(map[string]string{"TERM": "xterm-256color"})) {
		t.Fatal("terminal stderr should animate")
	}
}

func TestThemeOffIsPassthrough(t *testing.T) {
	for _, theme := range []Theme{{}, NewTheme(termenv.Ascii)} {
		if theme.Enabled() {
			t.Fatal("expected disabled theme")
		}
		if got := theme.Error("x") + theme.Muted("y") + theme.Strong("z"); got != "xyz" {
			t.Fatalf("disabled theme must not style text, got %q", got)
		}
	}
}

func TestThemeOnStylesText(t *testing.T) {
	theme := NewTheme(termenv.ANSI256)
	if got := theme.Success("ok"); !strings.Contains(got, "\x1b[") || !strings.Contains(got, "ok") {
		t.Fatalf("expected styled text, got %q", got)
	}
}

func TestGlyphSetsAreComplete(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		g := GlyphSet(ascii)
		for name, value := range map[string]string{
			"OK": g.OK, "Fail": g.Fail, "Warn": g.Warn, "Arrow": g.Arrow, "Step": g.Step,
			"SubStep": g.SubStep, "Pending": g.Pending, "Top": g.Top, "Rail": g.Rail,
			"Bottom": g.Bottom, "Dash": g.Dash, "Ellipsis": g.Ellipsis,
		} {
			if value == "" {
				t.Fatalf("ascii=%v: glyph %s is empty", ascii, name)
			}
			if ascii && strings.IndexFunc(value, func(r rune) bool { return r > 127 }) >= 0 {
				t.Fatalf("ascii glyph %s = %q is not ASCII", name, value)
			}
		}
		if len(g.Spinner) == 0 {
			t.Fatalf("ascii=%v: empty spinner", ascii)
		}
	}
}

func TestCopyTimelineHoldsSpinnerBackForShortStages(t *testing.T) {
	tl, buf := newAnimatedCopyTimelineForTest(80, false)
	tl.spinnerDelay = time.Hour

	tl.StageStart("Pages")
	tl.StageItem("Pages", "about", 1, 3)
	if strings.Contains(buf.String(), "⠋") {
		t.Fatalf("spinner shown before the delay:\n%q", buf.String())
	}

	tl.StageDone("Pages", "3 copied")
	if !strings.Contains(buf.String(), "◇ Pages — 3 copied") {
		t.Fatalf("expected completed stage line, got %q", buf.String())
	}
}

func TestCopyTimelineASCIIGlyphs(t *testing.T) {
	tl, buf := newStaticCopyTimelineForTest()
	tl.glyphs = GlyphSet(true)

	tl.Header("a", "b")
	tl.StageDone("Pages", "3 copied")
	tl.StageWarning("Pages", "slow")
	tl.Footer()

	out := buf.String()
	if strings.IndexFunc(out, func(r rune) bool { return r > 127 }) >= 0 {
		t.Fatalf("expected ASCII-only timeline, got:\n%s", out)
	}
	for _, want := range []string{"+ Copying a -> b", "* Pages - 3 copied", "|  ! slow", "+ Done!"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in:\n%s", want, out)
		}
	}
}

func TestSyncTimelineHoldsSpinnerBackForShortCategories(t *testing.T) {
	var buf strings.Builder
	tl := &SyncTimeline{
		writer:       &buf,
		glyphs:       GlyphSet(false),
		animated:     true,
		spinnerDelay: time.Hour,
		activeCat:    -1,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
	tl.SetCategories([]SyncCategory{{Label: "layouts", Count: 1}})
	tl.StartCategory(0)
	tl.SetActiveFile("layouts/default.liquid")
	if buf.Len() != 0 {
		t.Fatalf("spinner shown before the delay: %q", buf.String())
	}

	tl.spinnerDelay = 0
	tl.SetActiveFile("layouts/default.liquid")
	if !strings.Contains(buf.String(), "layouts — 0/1  layouts/default.liquid") {
		t.Fatalf("expected live line once the delay passed, got %q", buf.String())
	}
}
