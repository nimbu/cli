package cmd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func initTwoThemeServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sites":
			_, _ = w.Write([]byte(`[{"id":"site-1","subdomain":"demo-shop","name":"Demo Shop"},{"id":"site-2","subdomain":"other","name":"Other"}]`))
		case "/themes":
			_, _ = w.Write([]byte(`[{"id":"theme-1","short":"storefront","name":"Storefront"},{"id":"theme-2","short":"outlet","name":"Outlet"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// With --no-input every answer comes from flags and stdin is never read.
func TestInitNoInputRunsFromFlags(t *testing.T) {
	sourceDir := t.TempDir()
	writeInitStarterFixture(t, sourceDir)
	outputDir := t.TempDir()
	srv := initTwoThemeServer(t)

	ctx, stdout, _ := newInitTestContext(t, srv.URL, output.Mode{JSON: true})
	withTempCWD(t, t.TempDir(), func() {
		withTempStdin(t, "", func() {
			cmd := &InitCmd{Dir: sourceDir, OutputDir: outputDir, Theme: "outlet", Repeatables: "all", Bundles: "blog-articles", OnConflict: initConflictAsk}
			if err := cmd.Run(ctx, &RootFlags{NoInput: true, Site: "demo-shop"}); err != nil {
				t.Fatalf("run init: %v", err)
			}
		})
	})

	project, err := os.ReadFile(filepath.Join(outputDir, "theme-demo-shop", "nimbu.yml"))
	if err != nil {
		t.Fatalf("expected project in default directory: %v", err)
	}
	if !strings.Contains(string(project), "site: demo-shop") || !strings.Contains(string(project), "theme: outlet") {
		t.Fatalf("expected flag answers in nimbu.yml, got:\n%s", project)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "theme-demo-shop", "templates", "blog.liquid")); err != nil {
		t.Fatalf("expected --bundles to include the blog bundle: %v", err)
	}
	if !strings.Contains(stdout.String(), `"theme": "outlet"`) {
		t.Fatalf("expected JSON result, got %s", stdout.String())
	}
}

func TestInitFlagValuesAreValidated(t *testing.T) {
	sourceDir := t.TempDir()
	writeInitStarterFixture(t, sourceDir)
	srv := initTwoThemeServer(t)
	ctx, _, _ := newInitTestContext(t, srv.URL, output.Mode{JSON: true})

	cases := []struct {
		name string
		cmd  InitCmd
		site string
		want string
	}{
		{"unknown site", InitCmd{Theme: "outlet"}, "nope", `site "nope" not found`},
		{"unknown theme", InitCmd{Theme: "nope"}, "demo-shop", `theme "nope" not found`},
		{"unknown repeatable", InitCmd{Theme: "outlet", Repeatables: "header,footer"}, "demo-shop", `--repeatables: unknown id "footer"`},
		{"unknown bundle", InitCmd{Theme: "outlet", Bundles: "shop"}, "demo-shop", `--bundles: unknown id "shop"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTempCWD(t, t.TempDir(), func() {
				cmd := tc.cmd
				cmd.Dir = sourceDir
				cmd.OutputDir = t.TempDir()
				err := cmd.Run(ctx, &RootFlags{NoInput: true, Site: tc.site})
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("expected %q, got %v", tc.want, err)
				}
			})
		})
	}
}

func TestInitNoInputInPlaceConflictsNeedPolicy(t *testing.T) {
	sourceDir := t.TempDir()
	writeInitStarterFixture(t, sourceDir)
	srv := initTwoThemeServer(t)
	ctx, _, _ := newInitTestContext(t, srv.URL, output.Mode{JSON: true})

	target := t.TempDir()
	userConfig := "site: keep-me\ntheme: keep-me\n"
	if err := os.WriteFile(filepath.Join(target, "nimbu.yml"), []byte(userConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	err := (&InitCmd{Dir: sourceDir, Directory: target, Theme: "outlet"}).Run(ctx, &RootFlags{NoInput: true, Site: "demo-shop"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitUsage || !strings.Contains(err.Error(), "--on-conflict") || !strings.Contains(err.Error(), "nimbu.yml") {
		t.Fatalf("expected usage error naming --on-conflict and the conflict, got %v", err)
	}

	// (repeatables "all": the fixture's one-line repeatable blocks can't be removed)
	if err := (&InitCmd{Dir: sourceDir, Directory: target, Theme: "outlet", Repeatables: "all", OnConflict: initConflictKeep}).Run(ctx, &RootFlags{NoInput: true, Site: "demo-shop"}); err != nil {
		t.Fatalf("run init with --on-conflict keep: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(target, "nimbu.yml")); string(got) != userConfig {
		t.Fatalf("expected --on-conflict keep to preserve nimbu.yml, got:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(target, "package.json")); err != nil {
		t.Fatalf("expected non-conflicting files written: %v", err)
	}
}

// The line prompter must not ask about repeatables or bundles the template
// does not define.
func TestInitLinePromptSkipsEmptyManifestSections(t *testing.T) {
	sourceDir := t.TempDir()
	writeInitStarterFixture(t, sourceDir)
	manifest := "base_paths:\n  - nimbu.yml\n  - package.json\n"
	if err := os.WriteFile(filepath.Join(sourceDir, "bootstrap", "manifest.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(initSiteThemeHandler(t)))
	defer srv.Close()

	ctx, _, _ := newInitTestContext(t, srv.URL, output.Mode{JSON: true})
	outputDir := t.TempDir()
	prompts := captureStderr(t, func() {
		withTempCWD(t, t.TempDir(), func() {
			// site, directory, confirm — nothing else is asked.
			withTempStdin(t, strings.Join([]string{"", "", "y", ""}, "\n"), func() {
				if err := (&InitCmd{Dir: sourceDir, OutputDir: outputDir}).Run(ctx, &RootFlags{}); err != nil {
					t.Fatalf("run init: %v", err)
				}
			})
		})
	})
	for _, unwanted := range []string{"Repeatables", "Bundle"} {
		if strings.Contains(prompts, unwanted) {
			t.Fatalf("expected no %s prompt for a manifest without them, got:\n%s", unwanted, prompts)
		}
	}
	if _, err := os.Stat(filepath.Join(outputDir, "theme-demo-shop", "nimbu.yml")); err != nil {
		t.Fatalf("expected bootstrapped project: %v", err)
	}
}

func TestInitCloneEnvIsNonInteractive(t *testing.T) {
	env := func(vars map[string]string) []string {
		return initCloneEnv(func(key string) string { return vars[key] })
	}

	got := env(nil)
	for _, want := range []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_SSH_COMMAND=ssh -o BatchMode=yes"} {
		if !containsEnv(got, want) {
			t.Fatalf("expected %q in %v", want, got)
		}
	}
	if got := env(map[string]string{"GIT_SSH_COMMAND": "ssh -i ~/.ssh/work"}); !containsEnv(got, "GIT_SSH_COMMAND=ssh -i ~/.ssh/work -o BatchMode=yes") {
		t.Fatalf("expected existing GIT_SSH_COMMAND kept with BatchMode appended, got %v", got)
	}
	for _, item := range env(map[string]string{"GIT_SSH": "/usr/local/bin/plink"}) {
		if strings.HasPrefix(item, "GIT_SSH_COMMAND=") {
			t.Fatalf("expected a GIT_SSH wrapper to be left alone, got %v", item)
		}
	}
}

// Before the fix the SSH attempt ran with no env and could prompt for a
// passphrase or host key underneath the wizard.
func TestCloneStarterRepoEveryAttemptIsNonInteractive(t *testing.T) {
	t.Parallel()

	var calls []cloneCall
	runner := initCloneRunner{
		lookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
		run: func(_ context.Context, name string, args []string, env []string) error {
			calls = append(calls, cloneCall{Name: name, Args: append([]string(nil), args...), Env: append([]string(nil), env...)})
			if name == "gh" && args[0] == "auth" {
				return nil
			}
			return errors.New("fail")
		},
		getenv: func(string) string { return "" },
	}
	_ = cloneStarterRepoWithRunner(context.Background(), runner, filepath.Join(t.TempDir(), "src"), "zenjoy/theme-starterskit", "main")

	if len(calls) != 4 {
		t.Fatalf("expected gh auth, gh clone, ssh and https attempts, got %d", len(calls))
	}
	for _, call := range calls {
		if !containsEnv(call.Env, "GIT_TERMINAL_PROMPT=0") || !containsEnv(call.Env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes") {
			t.Fatalf("expected non-interactive env for %s %v, got %v", call.Name, call.Args, call.Env)
		}
	}
}

func TestCloneStarterRepoStopsWhenCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	runner := initCloneRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" {
				return "/usr/bin/git", nil
			}
			return "", errors.New("missing")
		},
		run: func(ctx context.Context, _ string, _ []string, _ []string) error {
			attempts++
			cancel() // the wizard is cancelled while git runs
			return ctx.Err()
		},
	}
	err := cloneStarterRepoWithRunner(ctx, runner, filepath.Join(t.TempDir(), "src"), "zenjoy/theme-starterskit", "main")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected no fallback attempts after cancel, got %d", attempts)
	}
}
