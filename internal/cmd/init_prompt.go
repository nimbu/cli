package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/bootstrap"
	"github.com/nimbu/cli/internal/output"
)

// errInitCancelled is returned when the user backs out of init.
var errInitCancelled = errors.New("init cancelled")

func newInitPrompter(ctx context.Context) initPrompter {
	var out io.Writer = os.Stderr
	if writer := output.WriterFromContext(ctx); writer != nil && writer.Err != nil {
		out = writer.Err
	}
	return lineInitPrompter{reader: bufio.NewReader(os.Stdin), out: out}
}

type lineInitPrompter struct {
	reader *bufio.Reader
	out    io.Writer
}

func (p lineInitPrompter) w() io.Writer {
	if p.out == nil {
		return os.Stderr
	}
	return p.out
}

func (p lineInitPrompter) ChooseSite(choices []initSiteChoice) (int, error) {
	for idx, choice := range choices {
		_, _ = fmt.Fprintf(p.w(), "[%d] %s\n", idx+1, choice.Label)
	}
	return promptChoiceWithReader(p.reader, "Site [1]: ", len(choices))
}

func (p lineInitPrompter) ChooseTheme(choices []initThemeChoice) (int, error) {
	for idx, choice := range choices {
		_, _ = fmt.Fprintf(p.w(), "[%d] %s\n", idx+1, choice.Label)
	}
	return promptChoiceWithReader(p.reader, "Theme [1]: ", len(choices))
}

func (p lineInitPrompter) DirectoryName(defaultName string) (string, error) {
	dir, err := promptWithReader(p.reader, fmt.Sprintf("Directory name [%s]: ", defaultName))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(dir) == "" {
		return defaultName, nil
	}
	return strings.TrimSpace(dir), nil
}

func (p lineInitPrompter) Repeatables(options []bootstrap.Repeatable) ([]string, error) {
	mode, err := promptWithReader(p.reader, "Repeatables [none|all|select] [none]: ")
	if err != nil {
		return nil, err
	}
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "", "none":
		return nil, nil
	case "all":
		return repeatableIDsForPrompt(options), nil
	case "select":
		_, _ = fmt.Fprintf(p.w(), "Repeatables: %s\n", joinRepeatableIDs(options))
		value, err := promptWithReader(p.reader, "Repeatable ids (comma-separated): ")
		if err != nil {
			return nil, err
		}
		return parseInitRepeatables(value, options)
	default:
		return nil, fmt.Errorf("invalid repeatables choice %q: use none, all or select", strings.TrimSpace(mode))
	}
}

func (p lineInitPrompter) Bundles(options []bootstrap.Bundle) ([]string, error) {
	_, _ = fmt.Fprintf(p.w(), "Bundles: %s\n", joinBundleIDs(options))
	value, err := promptWithReader(p.reader, "Bundle ids (comma-separated, blank for none): ")
	if err != nil {
		return nil, err
	}
	return parseInitBundles(value, options)
}

func (p lineInitPrompter) Confirm(source, outputPath string) (bool, error) {
	_, _ = fmt.Fprintf(p.w(), "Source: %s\nOutput: %s\n", source, outputPath)
	return promptConfirmWithReader(p.reader, "Create project")
}

// ResolveConflicts asks per file whether to overwrite an existing file, in the
// classic [y]es / [n]o / [a]ll style. It returns the set of source-relative
// paths the user declined to overwrite (kept as-is).
func (p lineInitPrompter) ResolveConflicts(conflicts []string) (map[string]struct{}, error) {
	skip := make(map[string]struct{})
	overwriteAll := false
	_, _ = fmt.Fprintf(p.w(), "%d file(s) already exist in the target directory.\n", len(conflicts))
	for _, rel := range conflicts {
		if overwriteAll {
			continue
		}
		value, err := promptWithReader(p.reader, fmt.Sprintf("Overwrite %s? [y/N/a]: ", rel))
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "a", "all":
			overwriteAll = true
		case "y", "yes":
			// overwrite this one
		default:
			skip[rel] = struct{}{}
		}
	}
	return skip, nil
}

func cloneStarterRepo(ctx context.Context, destDir, repo, branch string) error {
	return cloneStarterRepoWithRunner(ctx, defaultInitCloneRunner(), destDir, repo, branch)
}

type initCloneRunner struct {
	lookPath func(string) (string, error)
	run      func(ctx context.Context, name string, args []string, env []string) error
	getenv   func(string) string
}

type initCloneAttempt struct {
	name string
	run  func() error
}

func defaultInitCloneRunner() initCloneRunner {
	return initCloneRunner{
		lookPath: exec.LookPath,
		run: func(ctx context.Context, name string, args []string, env []string) error {
			// CommandContext kills git/gh when the wizard is cancelled, so a
			// clone never outlives the command that started it.
			cmd := exec.CommandContext(ctx, name, args...)
			// git's ssh/credential helpers can hold the output pipe open after
			// git is killed; stop waiting for them shortly after.
			cmd.WaitDelay = 2 * time.Second
			if len(env) > 0 {
				cmd.Env = append(os.Environ(), env...)
			}
			if output, err := cmd.CombinedOutput(); err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
			}
			return nil
		},
		getenv: os.Getenv,
	}
}

func cloneStarterRepoWithRunner(ctx context.Context, runner initCloneRunner, destDir, repo, branch string) error {
	if _, err := runner.lookPath("git"); err != nil {
		return fmt.Errorf("git is required for init")
	}

	repoKind := classifyStarterRepo(repo)
	attempts := initCloneAttemptsForRepo(ctx, runner, destDir, repo, branch, repoKind)
	var failures []string

	for _, attempt := range attempts {
		err := attempt.run()
		if err == nil {
			return nil
		}
		_ = os.RemoveAll(destDir)
		if ctxErr := ctx.Err(); ctxErr != nil {
			// Cancelled: stop here instead of trying the next transport.
			return fmt.Errorf("clone starterskit %s@%s: %w", repo, branch, ctxErr)
		}
		failures = append(failures, attempt.name+": "+err.Error())
	}

	return fmt.Errorf("clone starterskit %s@%s failed; use --dir, run 'gh auth login', or configure GitHub SSH (%s)", repo, branch, strings.Join(failures, "; "))
}

func initCloneAttemptsForRepo(ctx context.Context, runner initCloneRunner, destDir, repo, branch string, repoKind starterRepoKind) []initCloneAttempt {
	attempts := make([]initCloneAttempt, 0, 3)
	// Every attempt runs non-interactively: the wizard owns the terminal, so a
	// credential or host-key prompt from git, gh or ssh would hang under it.
	env := initCloneEnv(runner.getenv)

	if repoKind == starterRepoGitHubShorthand {
		if _, err := runner.lookPath("gh"); err == nil {
			if err := runner.run(ctx, "gh", []string{"auth", "status"}, env); err == nil {
				attempts = append(attempts, initCloneAttempt{
					name: "gh",
					run: func() error {
						return runner.run(ctx, "gh", []string{"repo", "clone", repo, destDir, "--", "--depth", "1", "--branch", branch}, env)
					},
				})
			}
		}

		sshURL, httpsURL := starterRepoGitHubURLs(repo)
		attempts = append(attempts,
			initGitCloneAttempt(ctx, runner, "ssh", sshURL, branch, destDir, env),
			initGitCloneAttempt(ctx, runner, "https", httpsURL, branch, destDir, env),
		)
		return attempts
	}

	switch repoKind {
	case starterRepoHTTPS:
		attempts = append(attempts, initGitCloneAttempt(ctx, runner, "https", repo, branch, destDir, env))
	case starterRepoSSH:
		attempts = append(attempts, initGitCloneAttempt(ctx, runner, "ssh", repo, branch, destDir, env))
	default:
		attempts = append(attempts, initGitCloneAttempt(ctx, runner, "git", repo, branch, destDir, env))
	}

	return attempts
}

func initGitCloneAttempt(ctx context.Context, runner initCloneRunner, name, repoURL, branch, destDir string, env []string) initCloneAttempt {
	args := []string{"clone", "--depth", "1", "--branch", branch, repoURL, destDir}
	return initCloneAttempt{
		name: name,
		run: func() error {
			return runner.run(ctx, "git", args, env)
		},
	}
}

type starterRepoKind int

const (
	starterRepoOther starterRepoKind = iota
	starterRepoGitHubShorthand
	starterRepoHTTPS
	starterRepoSSH
)

func classifyStarterRepo(repo string) starterRepoKind {
	trimmed := strings.TrimSpace(repo)
	switch {
	case strings.HasPrefix(trimmed, "https://"), strings.HasPrefix(trimmed, "http://"):
		return starterRepoHTTPS
	case strings.HasPrefix(trimmed, "git@"), strings.HasPrefix(trimmed, "ssh://"):
		return starterRepoSSH
	case strings.Count(trimmed, "/") == 1 && !strings.Contains(trimmed, "://") && !strings.Contains(trimmed, "@"):
		return starterRepoGitHubShorthand
	default:
		return starterRepoOther
	}
}

func starterRepoGitHubURLs(repo string) (sshURL string, httpsURL string) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(repo), ".git")
	return "git@github.com:" + trimmed + ".git", "https://github.com/" + trimmed + ".git"
}

// initCloneEnv disables every interactive prompt a clone could raise: git's
// credential prompt, Git Credential Manager's UI, and ssh's passphrase or
// host-key questions (BatchMode). An existing GIT_SSH_COMMAND is kept and gets
// BatchMode appended; a legacy GIT_SSH wrapper is left alone because
// GIT_SSH_COMMAND would silently override it.
func initCloneEnv(getenv func(string) string) []string {
	if getenv == nil {
		getenv = os.Getenv
	}
	env := []string{
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	}
	sshCommand := strings.TrimSpace(getenv("GIT_SSH_COMMAND"))
	switch {
	case sshCommand != "":
		if !strings.Contains(sshCommand, "BatchMode") {
			sshCommand += " -o BatchMode=yes"
		}
		env = append(env, "GIT_SSH_COMMAND="+sshCommand)
	case strings.TrimSpace(getenv("GIT_SSH")) == "":
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return env
}

func initSiteChoices(sites []api.Site) []initSiteChoice {
	out := make([]initSiteChoice, 0, len(sites))
	for _, site := range sites {
		label := strings.TrimSpace(site.Name)
		if label == "" {
			label = site.Subdomain
		}
		if site.Subdomain != "" {
			label += " (" + site.Subdomain + ")"
		}
		out = append(out, initSiteChoice{Label: label, Site: site})
	}
	return out
}

func initThemeChoices(themes []api.Theme) []initThemeChoice {
	out := make([]initThemeChoice, 0, len(themes))
	for _, theme := range themes {
		label := strings.TrimSpace(theme.Name)
		if label == "" {
			label = theme.ID
		}
		if theme.Short != "" {
			label += " (" + theme.Short + ")"
		} else if theme.ID != "" {
			label += " (" + theme.ID + ")"
		}
		out = append(out, initThemeChoice{Label: label, Theme: theme})
	}
	return out
}

func promptChoiceWithReader(reader *bufio.Reader, message string, max int) (int, error) {
	value, err := promptWithReader(reader, message)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	choice, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || choice < 1 || choice > max {
		return 0, fmt.Errorf("invalid choice")
	}
	return choice - 1, nil
}

func promptConfirmWithReader(reader *bufio.Reader, message string) (bool, error) {
	value, err := promptWithReader(reader, message+" [y/N]: ")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func parameterize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func splitCSVValues(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func joinBundleIDs(bundles []bootstrap.Bundle) string {
	values := make([]string, 0, len(bundles))
	for _, bundle := range bundles {
		values = append(values, bundle.ID)
	}
	return strings.Join(values, ", ")
}

func joinRepeatableIDs(repeatables []bootstrap.Repeatable) string {
	values := repeatableIDsForPrompt(repeatables)
	return strings.Join(values, ", ")
}

func repeatableIDsForPrompt(repeatables []bootstrap.Repeatable) []string {
	values := make([]string, 0, len(repeatables))
	for _, repeatable := range repeatables {
		values = append(values, repeatable.ID)
	}
	return values
}

func findSiteByID(sites []api.Site, id string) (api.Site, bool) {
	for _, site := range sites {
		if site.ID == id {
			return site, true
		}
	}
	return api.Site{}, false
}

func findThemeByID(themes []api.Theme, id string) (api.Theme, bool) {
	for _, theme := range themes {
		if theme.ID == id {
			return theme, true
		}
	}
	return api.Theme{}, false
}

// siteConfigValue is the identifier persisted as `site:` in nimbu.yml. The Nimbu
// API resolves the X-Nimbu-Site header by subdomain or ID, so we prefer the
// human-readable subdomain and fall back to the ID when it is unavailable.
func siteConfigValue(site api.Site) string {
	if value := strings.TrimSpace(site.Subdomain); value != "" {
		return value
	}
	return site.ID
}

// themeConfigValue is the identifier persisted as `theme:` in nimbu.yml. Theme
// API paths (/themes/<x>) accept the short slug or the ID, so we prefer the
// human-readable short slug, then the theme_short_id, and fall back to the ID.
// Name is intentionally excluded: it may contain spaces and makes a poor path
// segment.
func themeConfigValue(theme api.Theme) string {
	for _, candidate := range []string{theme.Short, theme.ThemeShortID} {
		if value := strings.TrimSpace(candidate); value != "" {
			return value
		}
	}
	return theme.ID
}
