package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

func TestThemePushOnlyAddsLocalLiquidDependencies(t *testing.T) {
	root := t.TempDir()
	writeThemeProject(t, root, map[string]string{
		"nimbu.yml":              "theme: demo\n",
		"templates/page.liquid":  "{% include 'svg/a' %}\n",
		"snippets/svg/a.liquid":  "{% include 'svg/b' %}\n",
		"snippets/svg/b.liquid":  "swoosh\n",
		"snippets/unused.liquid": "nope\n",
		"layouts/default.liquid": "layout\n",
	})
	t.Chdir(root)

	var uploaded []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode upload: %v", err)
		}
		name, _ := body["name"].(string)
		switch r.URL.Path {
		case "/themes/demo/templates":
			uploaded = append(uploaded, "templates/"+name)
		case "/themes/demo/snippets":
			uploaded = append(uploaded, "snippets/"+name)
		case "/themes/demo/layouts":
			uploaded = append(uploaded, "layouts/"+name)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": name, "code": body["code"]})
	}))
	defer srv.Close()

	ctx, _, errOut := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &ThemePushCmd{Only: []string{"templates/page.liquid"}}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("themes push --only: %v", err)
	}

	stderr := errOut.String()
	if !strings.Contains(stderr, "adding dependency snippets/svg/a.liquid (referenced by templates/page.liquid)") {
		t.Fatalf("stderr missing a.liquid add: %q", stderr)
	}
	if !strings.Contains(stderr, "adding dependency snippets/svg/b.liquid (referenced by snippets/svg/a.liquid)") {
		t.Fatalf("stderr missing b.liquid add: %q", stderr)
	}

	want := []string{"snippets/svg/a.liquid", "snippets/svg/b.liquid", "templates/page.liquid"}
	if !reflect.DeepEqual(uploaded, want) && !sameStrings(uploaded, want) {
		t.Fatalf("uploaded = %#v, want %#v", uploaded, want)
	}
	for _, name := range uploaded {
		if name == "snippets/unused.liquid" || name == "layouts/default.liquid" {
			t.Fatalf("unexpected extra upload: %#v", uploaded)
		}
	}
}

func TestWriteThemeTransferResultPrintsWarningsInJSONMode(t *testing.T) {
	ctx, out, errOut := newContractTestContext(t, "http://example.test", output.Mode{JSON: true})
	err := writeThemeTransferResult(ctx, themes.Result{
		Mode: "push",
		AddedDependencies: []themes.AddedDependency{
			{Path: "snippets/svg/a.liquid", ReferencedBy: "templates/page.liquid"},
		},
		Warnings: []string{"templates/page.liquid references snippets/missing.liquid, which is not in this theme"},
		Uploaded: []themes.Action{
			{DisplayPath: "snippets/svg/a.liquid", Dependency: true},
			{DisplayPath: "templates/page.liquid"},
		},
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("write result: %v", err)
	}
	if !strings.Contains(out.String(), `"dependency": true`) {
		t.Fatalf("stdout = %q", out.String())
	}
	stderr := errOut.String()
	if !strings.Contains(stderr, "adding dependency snippets/svg/a.liquid (referenced by templates/page.liquid)") {
		t.Fatalf("stderr missing add: %q", stderr)
	}
	if !strings.Contains(stderr, "warning: templates/page.liquid references snippets/missing.liquid, which is not in this theme") {
		t.Fatalf("stderr missing warning: %q", stderr)
	}
}

func TestWriteThemeTransferResultMarksDryRunDependencies(t *testing.T) {
	ctx, out, _ := newContractTestContext(t, "http://example.test", output.Mode{})
	err := writeThemeTransferResult(ctx, themes.Result{
		Mode:   "push",
		DryRun: true,
		Uploaded: []themes.Action{
			{DisplayPath: "snippets/svg/a.liquid", Dependency: true},
			{DisplayPath: "templates/page.liquid"},
		},
	})
	if err != nil {
		t.Fatalf("write result: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "[dry-run] upload snippets/svg/a.liquid (dependency)") {
		t.Fatalf("stdout = %q", got)
	}
	if !strings.Contains(got, "[dry-run] upload templates/page.liquid") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestWriteThemeTransferResultDryRunTimelineListsDependenciesOnce(t *testing.T) {
	ctx, out, errOut := newContractTestContext(t, "http://example.test", output.Mode{})
	err := writeThemeTransferResult(ctx, themes.Result{
		Mode:             "push",
		DryRun:           true,
		TimelineRendered: true,
		AddedDependencies: []themes.AddedDependency{
			{Path: "snippets/svg/a.liquid", ReferencedBy: "templates/page.liquid"},
		},
		Uploaded: []themes.Action{
			{DisplayPath: "snippets/svg/a.liquid", Dependency: true},
			{DisplayPath: "templates/page.liquid"},
		},
	})
	if err != nil {
		t.Fatalf("write result: %v", err)
	}
	got := out.String()
	if strings.Count(got, "[dry-run] upload snippets/svg/a.liquid (dependency)") != 1 {
		t.Fatalf("dependency list should appear once, stdout = %q", got)
	}
	if strings.Contains(got, "push complete") {
		t.Fatalf("timeline dry-run should not reprint the summary: %q", got)
	}
	if !strings.Contains(errOut.String(), "adding dependency snippets/svg/a.liquid (referenced by templates/page.liquid)") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestWriteThemeTransferResultJSONWithTimelineStaysJSONOnStdout(t *testing.T) {
	ctx, out, errOut := newContractTestContext(t, "http://example.test", output.Mode{JSON: true})
	err := writeThemeTransferResult(ctx, themes.Result{
		Mode:             "push",
		DryRun:           true,
		TimelineRendered: true,
		AddedDependencies: []themes.AddedDependency{
			{Path: "snippets/svg/a.liquid", ReferencedBy: "templates/page.liquid"},
		},
		Uploaded: []themes.Action{
			{DisplayPath: "snippets/svg/a.liquid", Dependency: true},
			{DisplayPath: "templates/page.liquid"},
		},
	})
	if err != nil {
		t.Fatalf("write result: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not a single JSON object: %v\n%s", err, out)
	}
	if _, ok := payload["added_dependencies"]; !ok {
		t.Fatalf("stdout = %s", out)
	}
	stderr := errOut.String()
	if !strings.Contains(stderr, "adding dependency snippets/svg/a.liquid (referenced by templates/page.liquid)") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(out.String(), "adding dependency") {
		t.Fatalf("notes leaked onto stdout: %s", out)
	}
}

func writeThemeProject(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	counts := map[string]int{}
	for _, value := range want {
		counts[value]++
	}
	for _, value := range got {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}
	return true
}
