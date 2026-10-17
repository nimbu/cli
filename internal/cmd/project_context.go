package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/envutil"
)

type projectContext struct {
	Config      config.ProjectConfig
	File        string
	ProjectRoot string
}

func resolveProjectContext() (projectContext, error) {
	projectFile, err := config.FindProjectFile()
	if err == nil {
		projectCfg, readErr := config.ReadProjectConfigFrom(projectFile)
		if readErr != nil {
			return projectContext{}, fmt.Errorf("read project config: %w", readErr)
		}
		return projectContext{
			Config:      projectCfg,
			File:        projectFile,
			ProjectRoot: filepath.Dir(projectFile),
		}, nil
	}
	if err != nil && !errors.Is(err, config.ErrNotFound) {
		return projectContext{}, err
	}
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		return projectContext{}, cwdErr
	}
	return projectContext{
		File:        filepath.Join(cwd, config.ProjectFileName),
		ProjectRoot: cwd,
	}, nil
}

func currentAPIHost(flags *RootFlags) string {
	if flags == nil {
		return ""
	}
	return apps.NormalizeHost(flags.APIURL)
}

// promptStdinIsTerminal reports whether stdin is an interactive terminal;
// swapped in tests.
var promptStdinIsTerminal = stdinIsTerminal

// ciEnvironment reports a CI runner (see envutil.IsCI), where nobody can
// answer a prompt.
func ciEnvironment() bool {
	return envutil.IsCI(os.Getenv)
}

// promptBlocker explains why a prompt cannot be shown, or "" when it can.
// --no-input, a CI runner and a non-terminal stdin all mean nobody can answer:
// reading anyway would fail with a bare EOF or consume piped data.
func promptBlocker(flags *RootFlags) string {
	switch {
	case flags != nil && flags.NoInput:
		return "--no-input"
	case ciEnvironment():
		return "CI"
	case !promptStdinIsTerminal():
		return "stdin is not a terminal"
	}
	return ""
}

// confirmPrompt asks a yes/no question that --force (or --yes) answers.
func confirmPrompt(flags *RootFlags, message string) (bool, error) {
	return confirmPromptFlag(flags, message, "--force")
}

// confirmPromptFlag asks a yes/no question. A caller that must not honor
// --force (protected environments) clears Force and Yes in flags and names
// its own override flag for the error.
func confirmPromptFlag(flags *RootFlags, message, overrideFlag string) (bool, error) {
	message = strings.TrimSpace(message)
	if confirmedByFlag(flags) {
		return true, nil
	}
	if reason := promptBlocker(flags); reason != "" {
		return false, confirmationRequiredError(
			fmt.Sprintf("refusing to %s without %s (cannot prompt: %s)", message, overrideFlag, reason),
			overrideFlag)
	}
	answer, err := prompt(fmt.Sprintf("%s? [y/N]: ", message))
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
