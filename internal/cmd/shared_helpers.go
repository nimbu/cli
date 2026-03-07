package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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

func projectFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, config.ProjectFileName)
}

func normalizeAPIHost(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("api url required")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	host := strings.TrimSpace(parsed.Host)
	if host == "" {
		return "", fmt.Errorf("invalid api url: %q", raw)
	}
	return strings.ToLower(host), nil
}

func newAPIClientForBase(ctx context.Context, baseURL string, site string) (*api.Client, error) {
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	token, err := ResolveAuthToken(ctx)
	if err != nil {
		return nil, err
	}

	client := api.New(baseURL, token)
	client = client.WithTimeout(flags.Timeout)
	client = client.WithDebug(flags.Debug)
	if site != "" {
		client = client.WithSite(site)
	}
	return client, nil
}

func promptYesNo(flags *RootFlags, message string) (bool, error) {
	if flags != nil && flags.NoInput {
		return false, fmt.Errorf("%s (use --force with --no-input)", message)
	}
	answer, err := prompt(message + " [y/N]: ")
	if err != nil {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}
