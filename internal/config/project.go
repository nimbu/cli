package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const ProjectFileName = ".nimbu.json"

// ProjectConfig holds project-specific configuration.
type ProjectConfig struct {
	Site  string `json:"site,omitempty"`
	Theme string `json:"theme,omitempty"`
}

// FindProjectFile walks up from the current directory to find .nimbu.json.
func FindProjectFile() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return findProjectFileFrom(dir)
}

func findProjectFileFrom(startDir string) (string, error) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, ProjectFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			return "", ErrNotFound
		}
		dir = parent
	}
}

// ReadProjectConfig reads the project config from .nimbu.json.
func ReadProjectConfig() (ProjectConfig, error) {
	path, err := FindProjectFile()
	if err != nil {
		return ProjectConfig{}, err
	}

	return ReadProjectConfigFrom(path)
}

// ReadProjectConfigFrom reads project config from a specific path.
func ReadProjectConfigFrom(path string) (ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectConfig{}, err
	}

	var cfg ProjectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ProjectConfig{}, err
	}

	return cfg, nil
}

// WriteProjectConfig writes project config to .nimbu.json in the current directory.
func WriteProjectConfig(cfg ProjectConfig) error {
	return WriteProjectConfigTo(ProjectFileName, cfg)
}

// WriteProjectConfigTo writes project config to a specific path.
func WriteProjectConfigTo(path string, cfg ProjectConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
