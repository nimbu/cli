package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/config"
)

func resolveProjectRoot() (string, config.ProjectConfig, error) {
	projectFile, err := config.FindProjectFile()
	if err == nil {
		cfg, readErr := config.ReadProjectConfigFrom(projectFile)
		if readErr != nil {
			return "", config.ProjectConfig{}, fmt.Errorf("read project config: %w", readErr)
		}
		return filepath.Dir(projectFile), cfg, nil
	}
	if err != nil && !errors.Is(err, config.ErrNotFound) {
		return "", config.ProjectConfig{}, err
	}

	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		return "", config.ProjectConfig{}, cwdErr
	}
	return cwd, config.ProjectConfig{}, nil
}

func newAPIClientForBase(ctx context.Context, baseURL string, site string) (*api.Client, error) {
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	token, err := ResolveAuthToken(ctx)
	if err != nil {
		return nil, err
	}

	client := api.New(baseURL, token)
	client = client.WithVersion(version)
	client = client.WithTimeout(flags.Timeout)
	client = client.WithDebug(flags.Debug)
	if site != "" {
		client = client.WithSite(site)
	}
	return client, nil
}
