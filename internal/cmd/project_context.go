package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/config"
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
	if err != nil && err != config.ErrNotFound {
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

func confirmPrompt(flags *RootFlags, message string) (bool, error) {
	if flags != nil && flags.Force {
		return true, nil
	}
	if flags != nil && flags.NoInput {
		return false, fmt.Errorf("use --force to confirm %s", strings.TrimSpace(message))
	}
	answer, err := prompt(fmt.Sprintf("%s [y/N]: ", message))
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
