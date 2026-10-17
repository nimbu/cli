package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/bootstrap"
)

var updateInitGolden = flag.Bool("update-init-golden", false, "rewrite internal/cmd/testdata/init_tui golden frames")

func initGoldenSites(n int) []api.Site {
	sites := make([]api.Site, 0, n)
	for i := 1; i <= n; i++ {
		sites = append(sites, api.Site{ID: fmt.Sprintf("s%d", i), Subdomain: fmt.Sprintf("site-%02d", i), Name: fmt.Sprintf("Customer site %02d", i)})
	}
	return sites
}

// initGoldenModel builds a wizard at a given step and terminal size.
func initGoldenModel(useColor bool, step string, width, height int) *initTeaModel {
	model := newInitTeaTestModel(initPromptModel{
		Sites:                initSiteChoices(initGoldenSites(25)),
		RepeatableOptions:    []bootstrap.Repeatable{{ID: "header", Label: "Header"}, {ID: "text", Label: "Text"}},
		BundleOptions:        []bootstrap.Bundle{{ID: "blog-articles", Label: "Blog + articles"}},
		DefaultDirectoryName: "theme-site-03",
		OutputDir:            "/work",
		Source:               "zenjoy/theme-starterskit@main",
	})
	model.useColor = useColor
	model.applyTheme()
	model.width, model.height = width, height
	model.resizeInputs()
	model.transcript = []initTranscriptEntry{{Label: "Template", Value: "Starterskit"}}

	switch step {
	case "site":
		model.enterStep(initTeaStepSite)
	case "site-scrolled":
		model.enterStep(initTeaStepSite)
		model.cursor = 16
	case "directory":
		model.transcript = append(model.transcript,
			initTranscriptEntry{Label: "Site", Value: "Customer site 03 (site-03)"},
			initTranscriptEntry{Label: "Theme", Value: "Storefront (storefront)"})
		model.enterStep(initTeaStepDirectory)
	case "confirm":
		model.transcript = append(model.transcript,
			initTranscriptEntry{Label: "Site", Value: "Customer site 03 (site-03)"},
			initTranscriptEntry{Label: "Theme", Value: "Storefront (storefront)"},
			initTranscriptEntry{Label: "Directory", Value: "theme-site-03"},
			initTranscriptEntry{Label: "Repeatables", Value: "header, text"},
			initTranscriptEntry{Label: "Bundles", Value: "blog-articles"})
		model.answers = initAnswers{ThemeID: "t1", DirectoryName: "theme-site-03", RepeatableIDs: []string{"header", "text"}, BundleIDs: []string{"blog-articles"}}
		model.prompt.Themes = initThemeChoices([]api.Theme{{ID: "t1", Name: "Storefront", Short: "storefront"}})
		model.enterStep(initTeaStepConfirm)
	case "loading":
		model.phase = initTeaPhaseLoading
		model.loadingSummary = "Site"
		model.loadingDetail = "Fetching your sites..."
	}
	return model
}

// initASCIIFrame renders the model the way Bubble Tea writes it with the color
// profile pinned to ASCII: colors dropped, attributes kept.
func initASCIIFrame(t *testing.T, model *initTeaModel) string {
	t.Helper()
	var buf bytes.Buffer
	w := &colorprofile.Writer{Forward: &buf, Profile: colorprofile.ASCII}
	if _, err := w.WriteString(initView(model)); err != nil {
		t.Fatalf("downsample frame: %v", err)
	}
	return buf.String()
}

func TestInitTeaGoldenFrames(t *testing.T) {
	cases := []struct {
		name          string
		step          string
		width, height int
	}{
		{"site_80x24", "site", 80, 24},
		{"site_scrolled_80x24", "site-scrolled", 80, 24},
		{"site_60x24", "site", 60, 24},
		{"site_min_40x12", "site", initMinWidth, initMinHeight},
		{"directory_60x24", "directory", 60, 24},
		{"confirm_80x24", "confirm", 80, 24},
		{"confirm_min_40x12", "confirm", initMinWidth, initMinHeight},
		{"loading_100x40", "loading", 100, 40},
		{"too_small_30x10", "site", 30, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frame := initASCIIFrame(t, initGoldenModel(true, tc.step, tc.width, tc.height))
			if strings.Contains(frame, "38;") || strings.Contains(frame, "48;") {
				t.Fatalf("ASCII profile frame still carries color sequences:\n%q", frame)
			}
			// Paths render with native separators; goldens are stored in slash form.
			got := filepath.ToSlash(ansi.Strip(frame)) + "\n"

			// Styling must never change layout: the no-color frame is identical.
			if plain := filepath.ToSlash(ansi.Strip(initView(initGoldenModel(false, tc.step, tc.width, tc.height)))) + "\n"; plain != got {
				t.Fatalf("color and no-color layouts differ\ncolor:\n%s\nplain:\n%s", got, plain)
			}

			path := filepath.Join("testdata", "init_tui", tc.name+".golden")
			if *updateInitGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (run with -update-init-golden): %v", err)
			}
			// A Windows checkout may turn the golden's LF into CRLF; the
			// comparison stays exact otherwise, so frame height is pinned.
			if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
				t.Fatalf("frame mismatch for %s\n--- got ---\n%s--- want ---\n%s", tc.name, got, want)
			}
		})
	}
}

// Every frame at or above the declared minimum fits the terminal: no line is
// wider than the width and the frame is never taller than the height, so the
// inline renderer never clips the top.
func TestInitTeaFramesFitTerminal(t *testing.T) {
	steps := []string{"site", "site-scrolled", "directory", "confirm", "loading"}
	for _, step := range steps {
		for width := initMinWidth; width <= 120; width += 7 {
			for height := initMinHeight; height <= 40; height += 3 {
				frame := initView(initGoldenModel(true, step, width, height))
				lines := strings.Split(frame, "\n")
				if len(lines) > height {
					t.Fatalf("%s at %dx%d: %d lines exceed height\n%s", step, width, height, len(lines), ansi.Strip(frame))
				}
				for _, line := range lines {
					if w := lipgloss.Width(line); w > width {
						t.Fatalf("%s at %dx%d: line width %d exceeds width: %q", step, width, height, w, ansi.Strip(line))
					}
				}
			}
		}
	}
}

func TestInitTeaTooSmallStateIsTruthful(t *testing.T) {
	model := initGoldenModel(false, "site", 30, 10)
	view := initView(model)
	for _, needle := range []string{"30×10", fmt.Sprintf("%d×%d", initMinWidth, initMinHeight), "esc"} {
		if !strings.Contains(strings.ReplaceAll(view, "\n", " "), needle) {
			t.Fatalf("expected too-small notice to mention %q, got:\n%s", needle, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 30 {
			t.Fatalf("too-small notice wider than terminal: %q", line)
		}
	}
}

func TestInitListWindowKeepsCursorVisibleWithinRows(t *testing.T) {
	for total := 1; total <= 30; total++ {
		for rows := initMinListRows; rows <= 12; rows++ {
			for cursor := 0; cursor < total; cursor++ {
				start, end := initListWindow(total, cursor, rows)
				if cursor < start || cursor >= end {
					t.Fatalf("total=%d rows=%d cursor=%d: window [%d,%d) hides cursor", total, rows, cursor, start, end)
				}
				used := end - start
				if start > 0 {
					used++
				}
				if end < total {
					used++
				}
				if used > rows {
					t.Fatalf("total=%d rows=%d cursor=%d: window [%d,%d) uses %d lines", total, rows, cursor, start, end, used)
				}
			}
		}
	}
}

// Truncation counts terminal cells, not runes, and keeps ANSI intact.
func TestInitTeaTruncatesByDisplayWidth(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{Sites: []initSiteChoice{
		{Label: "東京のお客様サイト東京のお客様サイト東京のお客様サイト (tokyo)"},
		{Label: "Emoji 🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉 site"},
	}})
	model.useColor = true
	model.applyTheme()
	model.width, model.height = 44, 20
	for _, line := range strings.Split(initView(model), "\n") {
		if w := lipgloss.Width(line); w > 44 {
			t.Fatalf("line width %d exceeds 44: %q", w, ansi.Strip(line))
		}
	}

	model.enterStep(initTeaStepDirectory)
	model.directoryInput.SetValue(strings.Repeat("長い名前", 20))
	line := renderLabelLine(model, "Directory", model.directoryInput.View(), 30)
	if w := lipgloss.Width(line); w > 30 {
		t.Fatalf("directory row width %d exceeds 30: %q", w, line)
	}
	if !strings.HasSuffix(ansi.Strip(line), "…") {
		t.Fatalf("expected truncated directory row to end in an ellipsis: %q", ansi.Strip(line))
	}
}

func TestInitTeaLightBackgroundSwitchesPalette(t *testing.T) {
	dark := newInitTheme(true, true)
	light := newInitTheme(true, false)
	if fmt.Sprint(dark.Muted) == fmt.Sprint(light.Muted) || fmt.Sprint(dark.Active) == fmt.Sprint(light.Active) {
		t.Fatal("expected chrome colors to differ between dark and light backgrounds")
	}
	if dark.Text != nil || light.Text != nil {
		t.Fatal("value text must keep the terminal's own foreground on both backgrounds")
	}

	model := initGoldenModel(true, "site", 80, 24)
	before := initView(model)
	model.Update(teaBackground(false))
	after := initView(model)
	if before == after {
		t.Fatal("expected a light background reply to restyle the frame")
	}
	if !strings.Contains(after, light.muted("┌")) {
		t.Fatalf("expected light muted color after background reply, got:\n%q", after)
	}
	if ansi.Strip(before) != ansi.Strip(after) {
		t.Fatal("expected the background reply to change colors only, not text")
	}
}

func TestInitTeaDefaultBannerUsesThemeTextColor(t *testing.T) {
	theme := newInitTheme(true, false)
	if got := theme.bannerLineColor(DefaultBannerPalette(), 0); got != nil {
		t.Fatalf("expected the default banner to keep the terminal's own text color, got %v", got)
	}
	rainbow, _ := BannerThemeByName("rainbow")
	if got := theme.bannerLineColor(rainbow.Palette, 0); fmt.Sprint(got) == fmt.Sprint(theme.Text) {
		t.Fatal("expected named banner themes to keep their own colors")
	}
}

func teaBackground(dark bool) tea.BackgroundColorMsg {
	if dark {
		return tea.BackgroundColorMsg{Color: lipgloss.Color("#000000")}
	}
	return tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")}
}
