package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/bootstrap"
)

func newInitPrompter(ctx context.Context) initPrompter {
	return lineInitPrompter{reader: bufio.NewReader(os.Stdin)}
}

type lineInitPrompter struct {
	reader *bufio.Reader
}

func (p lineInitPrompter) Run(model initPromptModel) (initAnswers, error) {
	answers := initAnswers{}
	if len(model.Themes) == 0 {
		for idx, choice := range model.Sites {
			fmt.Fprintf(os.Stderr, "[%d] %s\n", idx+1, choice.Label)
		}
		siteIndex, err := promptChoiceWithReader(p.reader, "Site [1]: ", len(model.Sites))
		if err != nil {
			return initAnswers{}, err
		}
		answers.SiteID = model.Sites[siteIndex].Site.ID
		return answers, nil
	}

	for idx, choice := range model.Themes {
		fmt.Fprintf(os.Stderr, "[%d] %s\n", idx+1, choice.Label)
	}
	themeIndex, err := promptChoiceWithReader(p.reader, "Theme [1]: ", len(model.Themes))
	if err != nil {
		return initAnswers{}, err
	}
	answers.ThemeID = model.Themes[themeIndex].Theme.ID

	dir, err := promptWithReader(p.reader, fmt.Sprintf("Directory name [%s]: ", model.DefaultDirectoryName))
	if err != nil {
		return initAnswers{}, err
	}
	if strings.TrimSpace(dir) == "" {
		dir = model.DefaultDirectoryName
	}
	answers.DirectoryName = dir

	repeatableMode, err := promptWithReader(p.reader, "Repeatables [none|all|select] [none]: ")
	if err != nil {
		return initAnswers{}, err
	}
	repeatableMode = strings.TrimSpace(strings.ToLower(repeatableMode))
	if repeatableMode == "" {
		repeatableMode = "none"
	}
	answers.RepeatableMode = repeatableMode

	if len(model.BundleOptions) > 0 {
		fmt.Fprintf(os.Stderr, "Bundles: %s\n", joinBundleIDs(model.BundleOptions))
		bundleValue, err := promptWithReader(p.reader, "Bundle ids (comma-separated, blank for none): ")
		if err != nil {
			return initAnswers{}, err
		}
		answers.BundleIDs = splitCSVValues(bundleValue)
	}

	if answers.RepeatableMode == "all" {
		answers.RepeatableIDs = repeatableIDsForPrompt(model.RepeatableOptions)
	}
	if answers.RepeatableMode == "select" {
		fmt.Fprintf(os.Stderr, "Repeatables: %s\n", joinRepeatableIDs(model.RepeatableOptions))
		repeatableValue, err := promptWithReader(p.reader, "Repeatable ids (comma-separated): ")
		if err != nil {
			return initAnswers{}, err
		}
		answers.RepeatableIDs = splitCSVValues(repeatableValue)
	}

	fmt.Fprintf(os.Stderr, "Source: %s\nOutput: %s\n", model.Source, filepath.Join(model.OutputDir, answers.DirectoryName))
	confirmed, err := promptConfirmWithReader(p.reader, "Create project")
	if err != nil {
		return initAnswers{}, err
	}
	if !confirmed {
		return initAnswers{}, errors.New("init cancelled")
	}
	answers.Confirmed = true
	return answers, nil
}

func cloneStarterRepo(destDir, repo, branch string) error {
	return cloneStarterRepoWithRunner(defaultInitCloneRunner(), destDir, repo, branch)
}

type initCloneRunner struct {
	lookPath func(string) (string, error)
	run      func(name string, args []string, env []string) error
}

type initCloneAttempt struct {
	name string
	run  func() error
}

func defaultInitCloneRunner() initCloneRunner {
	return initCloneRunner{
		lookPath: exec.LookPath,
		run: func(name string, args []string, env []string) error {
			cmd := exec.Command(name, args...)
			if len(env) > 0 {
				cmd.Env = append(os.Environ(), env...)
			}
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
			}
			return nil
		},
	}
}

func cloneStarterRepoWithRunner(runner initCloneRunner, destDir, repo, branch string) error {
	if _, err := runner.lookPath("git"); err != nil {
		return fmt.Errorf("git is required for init")
	}

	repoKind := classifyStarterRepo(repo)
	attempts := initCloneAttemptsForRepo(runner, destDir, repo, branch, repoKind)
	var failures []string

	for _, attempt := range attempts {
		if err := attempt.run(); err == nil {
			return nil
		} else {
			failures = append(failures, attempt.name+": "+err.Error())
		}
		_ = os.RemoveAll(destDir)
	}

	return fmt.Errorf("clone starterskit %s@%s failed; use --dir, run 'gh auth login', or configure GitHub SSH (%s)", repo, branch, strings.Join(failures, "; "))
}

func initCloneAttemptsForRepo(runner initCloneRunner, destDir, repo, branch string, repoKind starterRepoKind) []initCloneAttempt {
	attempts := make([]initCloneAttempt, 0, 3)

	if repoKind == starterRepoGitHubShorthand {
		if _, err := runner.lookPath("gh"); err == nil {
			if err := runner.run("gh", []string{"auth", "status"}, nil); err == nil {
				attempts = append(attempts, initCloneAttempt{
					name: "gh",
					run: func() error {
						return runner.run("gh", []string{"repo", "clone", repo, destDir, "--", "--depth", "1", "--branch", branch}, nil)
					},
				})
			}
		}

		sshURL, httpsURL := starterRepoGitHubURLs(repo)
		attempts = append(attempts,
			initGitCloneAttempt(runner, "ssh", sshURL, branch, destDir, nil),
			initGitCloneAttempt(runner, "https", httpsURL, branch, destDir, initHTTPSCloneEnv()),
		)
		return attempts
	}

	switch repoKind {
	case starterRepoHTTPS:
		attempts = append(attempts, initGitCloneAttempt(runner, "https", repo, branch, destDir, initHTTPSCloneEnv()))
	case starterRepoSSH:
		attempts = append(attempts, initGitCloneAttempt(runner, "ssh", repo, branch, destDir, nil))
	default:
		attempts = append(attempts, initGitCloneAttempt(runner, "git", repo, branch, destDir, nil))
	}

	return attempts
}

func initGitCloneAttempt(runner initCloneRunner, name, repoURL, branch, destDir string, env []string) initCloneAttempt {
	args := []string{"clone", "--depth", "1", "--branch", branch, repoURL, destDir}
	return initCloneAttempt{
		name: name,
		run: func() error {
			return runner.run("git", args, env)
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

func initHTTPSCloneEnv() []string {
	return []string{
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	}
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
