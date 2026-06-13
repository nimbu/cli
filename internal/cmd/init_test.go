package cmd

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestInitRejectsNoInput(t *testing.T) {
	ctx, _, _ := newInitTestContext(t, "https://api.example.test", output.Mode{})
	cmd := &InitCmd{}

	err := cmd.Run(ctx, &RootFlags{NoInput: true})
	if err == nil || !strings.Contains(err.Error(), "interactive only") {
		t.Fatalf("expected interactive-only error, got %v", err)
	}
}

func TestInitBootstrapsProjectIntoOutputDir(t *testing.T) {
	sourceDir := t.TempDir()
	writeInitStarterFixture(t, sourceDir)
	outputDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sites":
			_, _ = w.Write([]byte(`[{"id":"site-1","subdomain":"demo-shop","name":"Demo Shop"}]`))
		case "/themes":
			if got := r.Header.Get("X-Nimbu-Site"); got != "site-1" {
				t.Fatalf("expected site header site-1, got %q", got)
			}
			_, _ = w.Write([]byte(`[{"id":"storefront","name":"Storefront"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, stdout, _ := newInitTestContext(t, srv.URL, output.Mode{JSON: true})
	withTempCWD(t, t.TempDir(), func() {
		withTempStdin(t, strings.Join([]string{
			"",
			"",
			"",
			"select",
			"blog-articles",
			"header,text",
			"y",
			"",
		}, "\n"), func() {
			cmd := &InitCmd{
				Dir:       sourceDir,
				OutputDir: outputDir,
			}
			if err := cmd.Run(ctx, &RootFlags{}); err != nil {
				t.Fatalf("run init: %v", err)
			}
		})
	})

	finalPath := filepath.Join(outputDir, "theme-demo-shop")
	if _, err := os.Stat(filepath.Join(finalPath, "nimbu.yml")); err != nil {
		t.Fatalf("expected bootstrapped project: %v", err)
	}
	// JSON encodes backslashes as \\, so use forward slashes for comparison.
	normalizedOutput := strings.ReplaceAll(stdout.String(), `\\`, `/`)
	normalizedPath := filepath.ToSlash(finalPath)
	if !strings.Contains(normalizedOutput, `"path": `) || !strings.Contains(normalizedOutput, normalizedPath) {
		t.Fatalf("expected json output with final path, got %s", stdout.String())
	}
}

func TestInitInPlaceLeavesGitAloneAndHonorsConflictChoice(t *testing.T) {
	sourceDir := t.TempDir()
	writeInitStarterFixture(t, sourceDir)

	srv := httptest.NewServer(http.HandlerFunc(initSiteThemeHandler(t)))
	defer srv.Close()

	ctx, stdout, _ := newInitTestContext(t, srv.URL, output.Mode{JSON: true})
	target := t.TempDir()
	// Existing repo (no commits) and a user-authored nimbu.yml in the target.
	runGitForTest(t, target, "init")
	if err := os.WriteFile(filepath.Join(target, "nimbu.yml"), []byte("site: keep-me\ntheme: keep-me\n"), 0o644); err != nil {
		t.Fatalf("write user nimbu.yml: %v", err)
	}

	withTempCWD(t, target, func() {
		// site, theme, repeatable-mode (all), bundle-ids, confirm, then overwrite nimbu.yml -> "n" (keep mine)
		withTempStdin(t, strings.Join([]string{"", "", "all", "", "y", "n"}, "\n"), func() {
			cmd := &InitCmd{Dir: sourceDir, Directory: "."}
			if err := cmd.Run(ctx, &RootFlags{}); err != nil {
				t.Fatalf("run init: %v", err)
			}
		})
	})

	// Scaffolded in place: no nested directory, files land in target.
	if _, err := os.Stat(filepath.Join(target, "package.json")); err != nil {
		t.Fatalf("expected package.json scaffolded in place: %v", err)
	}
	// Declined conflict preserved and not rewritten.
	project, err := os.ReadFile(filepath.Join(target, "nimbu.yml"))
	if err != nil {
		t.Fatalf("read nimbu.yml: %v", err)
	}
	if !strings.Contains(string(project), "site: keep-me") || strings.Contains(string(project), "site-1") {
		t.Fatalf("expected user nimbu.yml preserved, got:\n%s", project)
	}
	// Git left alone: repo intact, no commit created.
	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Fatalf("expected existing .git preserved: %v", err)
	}
	if count := strings.TrimSpace(runGitForTest(t, target, "rev-list", "--all", "--count")); count != "0" {
		t.Fatalf("expected no commit in existing repo, got %q", count)
	}
	// JSON output reports the in-place target path.
	normalized := strings.ReplaceAll(stdout.String(), `\\`, `/`)
	if !strings.Contains(normalized, filepath.ToSlash(target)) {
		t.Fatalf("expected output path %q, got %s", target, stdout.String())
	}
}

func TestInitPositionalNewPathCreatesAndCommits(t *testing.T) {
	sourceDir := t.TempDir()
	writeInitStarterFixture(t, sourceDir)

	srv := httptest.NewServer(http.HandlerFunc(initSiteThemeHandler(t)))
	defer srv.Close()

	ctx, _, _ := newInitTestContext(t, srv.URL, output.Mode{JSON: true})
	target := filepath.Join(t.TempDir(), "new-theme")

	withTempCWD(t, t.TempDir(), func() {
		// site, theme, repeatable-mode (all), bundle-ids, confirm (no conflict prompt — fresh path)
		withTempStdin(t, strings.Join([]string{"", "", "all", "", "y"}, "\n"), func() {
			cmd := &InitCmd{Dir: sourceDir, Directory: target}
			if err := cmd.Run(ctx, &RootFlags{}); err != nil {
				t.Fatalf("run init: %v", err)
			}
		})
	})

	// Fresh path created with rewritten config and an initial commit.
	project, err := os.ReadFile(filepath.Join(target, "nimbu.yml"))
	if err != nil {
		t.Fatalf("read nimbu.yml: %v", err)
	}
	if !strings.Contains(string(project), "site: site-1") {
		t.Fatalf("expected nimbu.yml rewritten with selected site, got:\n%s", project)
	}
	if count := strings.TrimSpace(runGitForTest(t, target, "rev-list", "--all", "--count")); count == "0" {
		t.Fatalf("expected an initial commit in fresh project, got %q", count)
	}
}

func TestLineInitPrompterResolveConflicts(t *testing.T) {
	// y → overwrite, n → keep (skip), a → overwrite all remaining (no further prompts).
	p := lineInitPrompter{reader: bufio.NewReader(strings.NewReader("y\nn\na\n"))}
	skip, err := p.ResolveConflicts([]string{"a.txt", "b.txt", "c.txt", "d.txt"})
	if err != nil {
		t.Fatalf("resolve conflicts: %v", err)
	}
	if _, ok := skip["b.txt"]; !ok {
		t.Fatalf("expected b.txt declined (skipped), got %v", skip)
	}
	for _, overwritten := range []string{"a.txt", "c.txt", "d.txt"} {
		if _, ok := skip[overwritten]; ok {
			t.Fatalf("expected %s to be overwritten (not skipped), got %v", overwritten, skip)
		}
	}
	if len(skip) != 1 {
		t.Fatalf("expected exactly one declined file, got %v", skip)
	}
}

func TestLineInitPrompterResolveConflictsDefaultsToKeep(t *testing.T) {
	// Blank and unrecognized answers default to keeping the user's file.
	p := lineInitPrompter{reader: bufio.NewReader(strings.NewReader("\nmaybe\n"))}
	skip, err := p.ResolveConflicts([]string{"x", "y"})
	if err != nil {
		t.Fatalf("resolve conflicts: %v", err)
	}
	if len(skip) != 2 {
		t.Fatalf("expected both files kept (skipped), got %v", skip)
	}
}

func TestTargetIsExistingDir(t *testing.T) {
	dir := t.TempDir()

	if in, err := targetIsExistingDir(true, dir); err != nil || !in {
		t.Fatalf("existing dir: expected (true,nil), got (%v,%v)", in, err)
	}
	if in, err := targetIsExistingDir(true, filepath.Join(dir, "missing")); err != nil || in {
		t.Fatalf("missing path: expected (false,nil), got (%v,%v)", in, err)
	}

	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if in, err := targetIsExistingDir(true, file); err == nil || in {
		t.Fatalf("file target: expected (false,error), got (%v,%v)", in, err)
	}
	if in, err := targetIsExistingDir(false, file); err != nil || in {
		t.Fatalf("not fixed: expected (false,nil), got (%v,%v)", in, err)
	}
}

func initSiteThemeHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sites":
			_, _ = w.Write([]byte(`[{"id":"site-1","subdomain":"demo-shop","name":"Demo Shop"}]`))
		case "/themes":
			_, _ = w.Write([]byte(`[{"id":"storefront","name":"Storefront"}]`))
		default:
			http.NotFound(w, r)
		}
	}
}

func runGitForTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func TestCloneStarterRepoPrefersGHForGitHubShorthand(t *testing.T) {
	t.Parallel()

	destDir := filepath.Join(t.TempDir(), "source")
	var calls []cloneCall
	runner := initCloneRunner{
		lookPath: func(name string) (string, error) {
			switch name {
			case "git", "gh":
				return "/usr/bin/" + name, nil
			default:
				return "", errors.New("missing")
			}
		},
		run: func(name string, args []string, env []string) error {
			calls = append(calls, cloneCall{Name: name, Args: append([]string(nil), args...), Env: append([]string(nil), env...)})
			return nil
		},
	}

	if err := cloneStarterRepoWithRunner(runner, destDir, "zenjoy/theme-starterskit", "vite-go-cli"); err != nil {
		t.Fatalf("cloneStarterRepoWithRunner: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 gh calls, got %d", len(calls))
	}
	if calls[0].Name != "gh" || !reflect.DeepEqual(calls[0].Args, []string{"auth", "status"}) {
		t.Fatalf("expected gh auth status first, got %+v", calls[0])
	}
	expectedClone := []string{"repo", "clone", "zenjoy/theme-starterskit", destDir, "--", "--depth", "1", "--branch", "vite-go-cli"}
	if calls[1].Name != "gh" || !reflect.DeepEqual(calls[1].Args, expectedClone) {
		t.Fatalf("expected gh repo clone call, got %+v", calls[1])
	}
}

func TestCloneStarterRepoFallsBackToSSHThenHTTPS(t *testing.T) {
	t.Parallel()

	destDir := filepath.Join(t.TempDir(), "source")
	var calls []cloneCall
	runner := initCloneRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" {
				return "/usr/bin/git", nil
			}
			return "", errors.New("missing")
		},
		run: func(name string, args []string, env []string) error {
			calls = append(calls, cloneCall{Name: name, Args: append([]string(nil), args...), Env: append([]string(nil), env...)})
			switch len(calls) {
			case 1:
				if err := os.MkdirAll(destDir, 0o755); err != nil {
					t.Fatalf("mkdir dest: %v", err)
				}
				if err := os.WriteFile(filepath.Join(destDir, "leftover.txt"), []byte("x"), 0o644); err != nil {
					t.Fatalf("write leftover: %v", err)
				}
				return errors.New("ssh failed")
			case 2:
				if _, err := os.Stat(filepath.Join(destDir, "leftover.txt")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("expected dest dir cleanup before https fallback, stat err=%v", err)
				}
				return nil
			default:
				return nil
			}
		},
	}

	if err := cloneStarterRepoWithRunner(runner, destDir, "zenjoy/theme-starterskit", "vite-go-cli"); err != nil {
		t.Fatalf("cloneStarterRepoWithRunner: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 git clone attempts, got %d", len(calls))
	}
	if got := calls[0].Args[4]; got != "vite-go-cli" {
		t.Fatalf("expected branch arg in position 4, got %q", got)
	}
	if got := calls[0].Args[5]; got != "git@github.com:zenjoy/theme-starterskit.git" {
		t.Fatalf("expected ssh clone first, got %q", got)
	}
	if got := calls[1].Args[5]; got != "https://github.com/zenjoy/theme-starterskit.git" {
		t.Fatalf("expected https clone second, got %q", got)
	}
	if !containsEnv(calls[1].Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("expected https fallback to disable git terminal prompt, env=%v", calls[1].Env)
	}
}

func TestCloneStarterRepoExplicitHTTPSDisablesPrompts(t *testing.T) {
	t.Parallel()

	var calls []cloneCall
	runner := initCloneRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" {
				return "/usr/bin/git", nil
			}
			return "", errors.New("missing")
		},
		run: func(name string, args []string, env []string) error {
			calls = append(calls, cloneCall{Name: name, Args: append([]string(nil), args...), Env: append([]string(nil), env...)})
			return nil
		},
	}

	if err := cloneStarterRepoWithRunner(runner, t.TempDir(), "https://github.com/zenjoy/theme-starterskit.git", "vite-go-cli"); err != nil {
		t.Fatalf("cloneStarterRepoWithRunner: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected single https git clone, got %d", len(calls))
	}
	if calls[0].Name != "git" {
		t.Fatalf("expected git clone call, got %+v", calls[0])
	}
	if !containsEnv(calls[0].Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("expected non-interactive https env, got %v", calls[0].Env)
	}
}

func TestCloneStarterRepoFailureIsActionable(t *testing.T) {
	t.Parallel()

	runner := initCloneRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" {
				return "/usr/bin/git", nil
			}
			return "", errors.New("missing")
		},
		run: func(name string, args []string, env []string) error {
			return errors.New("boom")
		},
	}

	err := cloneStarterRepoWithRunner(runner, filepath.Join(t.TempDir(), "source"), "zenjoy/theme-starterskit", "vite-go-cli")
	if err == nil {
		t.Fatalf("expected clone failure")
	}
	for _, needle := range []string{"--dir", "gh auth login", "GitHub SSH"} {
		if !strings.Contains(err.Error(), needle) {
			t.Fatalf("expected actionable clone error containing %q, got %v", needle, err)
		}
	}
}

func TestLoadInitManifestMissingIsActionable(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	_, err := loadInitManifest(sourceDir, "zenjoy/theme-starterskit@vite-go-cli")
	if err == nil {
		t.Fatal("expected missing manifest error")
	}
	for _, needle := range []string{
		filepath.Join("bootstrap", "manifest.yml"),
		"zenjoy/theme-starterskit@vite-go-cli",
		"--dir",
		"bootstrap-ready",
	} {
		if !strings.Contains(err.Error(), needle) {
			t.Fatalf("expected %q in error, got %v", needle, err)
		}
	}
}

func TestInitShowsSourcePreparationProgressBeforePrompting(t *testing.T) {
	sourceDir := t.TempDir()
	writeInitStarterFixture(t, sourceDir)
	outputDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sites":
			_, _ = w.Write([]byte(`[{"id":"site-1","subdomain":"demo-shop","name":"Demo Shop"}]`))
		case "/themes":
			_, _ = w.Write([]byte(`[{"id":"storefront","name":"Storefront"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, stderr := newInitTestContext(t, srv.URL, output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   &strings.Builder{},
		Err:   stderr,
		Mode:  output.Mode{},
		NoTTY: true,
	})
	ctx = output.WithProgress(ctx, output.NewProgress(ctx))

	withTempCWD(t, t.TempDir(), func() {
		withTempStdin(t, strings.Join([]string{
			"",
			"",
			"",
			"select",
			"blog-articles",
			"header,text",
			"y",
			"",
		}, "\n"), func() {
			cmd := &InitCmd{
				Dir:       sourceDir,
				OutputDir: outputDir,
			}
			if err := cmd.Run(ctx, &RootFlags{}); err != nil {
				t.Fatalf("run init: %v", err)
			}
		})
	})

	for _, needle := range []string{
		"███╗   ██╗██╗███╗   ███╗██████╗ ██╗   ██╗",
		"Loading bootstrap manifest...",
		"done  Loading bootstrap manifest",
	} {
		if !strings.Contains(stderr.String(), needle) {
			t.Fatalf("expected progress output containing %q, got:\n%s", needle, stderr.String())
		}
	}
	if strings.Index(stderr.String(), "███╗   ██╗██╗███╗   ███╗██████╗ ██╗   ██╗") > strings.Index(stderr.String(), "Loading bootstrap manifest...") {
		t.Fatalf("expected banner before progress output, got:\n%s", stderr.String())
	}
}

type cloneCall struct {
	Name string
	Args []string
	Env  []string
}

func containsEnv(env []string, needle string) bool {
	for _, item := range env {
		if item == needle {
			return true
		}
	}
	return false
}

func newInitTestContext(t *testing.T, apiURL string, mode output.Mode) (context.Context, *strings.Builder, *strings.Builder) {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	flags := &RootFlags{APIURL: apiURL, Site: "site-1"}
	cfg := config.Defaults()
	cfg.DefaultSite = "site-1"

	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, mode)
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   stdout,
		Err:   stderr,
		Mode:  mode,
		NoTTY: true,
	})
	return ctx, stdout, stderr
}

func writeInitStarterFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "bootstrap"), 0o755); err != nil {
		t.Fatalf("mkdir bootstrap: %v", err)
	}
	manifest := strings.TrimSpace(`
base_paths:
  - nimbu.yml
  - package.json
  - templates/page.liquid
bundles:
  - id: blog-articles
    label: Blog + articles
    paths:
      - templates/blog.liquid
      - templates/article.liquid
repeatables:
  - id: header
    label: Header
    paths:
      - snippets/repeatables/header.liquid
    transforms:
      - type: remove_repeatable
        path: templates/page.liquid
        name: header
  - id: text
    label: Text
    paths:
      - snippets/repeatables/text.liquid
    transforms:
      - type: remove_repeatable
        path: templates/page.liquid
        name: text
`)
	if err := os.WriteFile(filepath.Join(root, "bootstrap", "manifest.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "nimbu.yml"), []byte("site: old\ntheme: old\n"), 0o644); err != nil {
		t.Fatalf("write nimbu.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "templates", "page.liquid"), []byte(`{% repeatable "header", label: "Header" %}{% endrepeatable %}
{% repeatable "text", label: "Text" %}{% endrepeatable %}
`), 0o644); err != nil {
		t.Fatalf("write page.liquid: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "templates", "blog.liquid"), []byte("blog\n"), 0o644); err != nil {
		t.Fatalf("write blog.liquid: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "templates", "article.liquid"), []byte("article\n"), 0o644); err != nil {
		t.Fatalf("write article.liquid: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "snippets", "repeatables"), 0o755); err != nil {
		t.Fatalf("mkdir snippets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "snippets", "repeatables", "header.liquid"), []byte("header\n"), 0o644); err != nil {
		t.Fatalf("write header snippet: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "snippets", "repeatables", "text.liquid"), []byte("text\n"), 0o644); err != nil {
		t.Fatalf("write text snippet: %v", err)
	}
}
