package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const ProjectFileName = "nimbu.yml"

// ProjectConfig holds project-specific configuration.
type ProjectConfig struct {
	Site  string      `json:"site,omitempty" yaml:"site,omitempty"`
	Theme string      `json:"theme,omitempty" yaml:"theme,omitempty"`
	Dev   *DevConfig  `json:"dev,omitempty" yaml:"dev,omitempty"`
	Sync  *SyncConfig `json:"sync,omitempty" yaml:"sync,omitempty"`
}

// DevConfig holds local development server configuration.
type DevConfig struct {
	Proxy  DevProxyConfig  `json:"proxy,omitempty" yaml:"proxy,omitempty"`
	Routes DevRoutesConfig `json:"routes,omitempty" yaml:"routes,omitempty"`
	Server DevServerConfig `json:"server,omitempty" yaml:"server,omitempty"`
}

// DevProxyConfig configures the local Nimbu proxy runtime.
type DevProxyConfig struct {
	Host              string `json:"host,omitempty" yaml:"host,omitempty"`
	MaxBodyMB         int    `json:"max_body_mb,omitempty" yaml:"max_body_mb,omitempty"`
	Port              int    `json:"port,omitempty" yaml:"port,omitempty"`
	TemplateRoot      string `json:"template_root,omitempty" yaml:"template_root,omitempty"`
	Watch             *bool  `json:"watch,omitempty" yaml:"watch,omitempty"`
	WatchScanInterval string `json:"watch_scan_interval,omitempty" yaml:"watch_scan_interval,omitempty"`
}

// DevRoutesConfig configures include/exclude path rules for proxy routing.
//
// Each rule accepts either:
// - "<glob>", e.g. "/**", "/account/*"
// - "<METHOD> <glob>", e.g. "POST /.well-known/*"
type DevRoutesConfig struct {
	Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
	Include []string `json:"include,omitempty" yaml:"include,omitempty"`
}

// DevServerConfig configures the child development server process.
type DevServerConfig struct {
	Args     []string          `json:"args,omitempty" yaml:"args,omitempty"`
	CWD      string            `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	Command  string            `json:"command,omitempty" yaml:"command,omitempty"`
	Env      map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	ReadyURL string            `json:"ready_url,omitempty" yaml:"ready_url,omitempty"`
}

// SyncConfig configures theme push/sync behavior.
type SyncConfig struct {
	Build     SyncBuildConfig `json:"build,omitempty" yaml:"build,omitempty"`
	Generated []string        `json:"generated,omitempty" yaml:"generated,omitempty"`
	Ignore    []string        `json:"ignore,omitempty" yaml:"ignore,omitempty"`
	Roots     SyncRootsConfig `json:"roots,omitempty" yaml:"roots,omitempty"`
}

// SyncBuildConfig configures the optional build step for theme push/sync.
type SyncBuildConfig struct {
	Args    []string          `json:"args,omitempty" yaml:"args,omitempty"`
	CWD     string            `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	Command string            `json:"command,omitempty" yaml:"command,omitempty"`
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

// SyncRootsConfig groups local directories by remote theme resource kind.
type SyncRootsConfig struct {
	Assets    []string `json:"assets,omitempty" yaml:"assets,omitempty"`
	Layouts   []string `json:"layouts,omitempty" yaml:"layouts,omitempty"`
	Snippets  []string `json:"snippets,omitempty" yaml:"snippets,omitempty"`
	Templates []string `json:"templates,omitempty" yaml:"templates,omitempty"`
}

// FindProjectFile walks up from the current directory to find nimbu.yml.
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

// ProjectRoot returns the directory containing the nearest nimbu.yml.
func ProjectRoot() (string, error) {
	path, err := FindProjectFile()
	if err != nil {
		return "", err
	}

	return filepath.Dir(path), nil
}

// ReadProjectConfig reads the project config from nimbu.yml.
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
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ProjectConfig{}, err
	}

	return cfg, nil
}

// WarnUnknownDevKeys returns warning strings for unknown keys in the `dev` block.
// Unknown keys are non-fatal and should be surfaced to users at startup.
func WarnUnknownDevKeys(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	devRaw, ok := root["dev"]
	if !ok || devRaw == nil {
		return nil, nil
	}

	dev, ok := asStringMap(devRaw)
	if !ok {
		return nil, nil
	}

	var warnings []string
	warnings = appendUnknownMapKeys(warnings, "dev", dev, map[string]struct{}{
		"proxy":  {},
		"routes": {},
		"server": {},
	})

	if raw, ok := dev["proxy"]; ok {
		if proxy, ok := asStringMap(raw); ok {
			warnings = appendUnknownMapKeys(warnings, "dev.proxy", proxy, map[string]struct{}{
				"host":                {},
				"max_body_mb":         {},
				"port":                {},
				"template_root":       {},
				"watch":               {},
				"watch_scan_interval": {},
			})
		}
	}

	if raw, ok := dev["server"]; ok {
		if server, ok := asStringMap(raw); ok {
			warnings = appendUnknownMapKeys(warnings, "dev.server", server, map[string]struct{}{
				"args":      {},
				"command":   {},
				"cwd":       {},
				"env":       {},
				"ready_url": {},
			})
		}
	}

	if raw, ok := dev["routes"]; ok {
		if routes, ok := asStringMap(raw); ok {
			warnings = appendUnknownMapKeys(warnings, "dev.routes", routes, map[string]struct{}{
				"exclude": {},
				"include": {},
			})
		}
	}

	return warnings, nil
}

// WarnUnknownSyncKeys returns warning strings for unknown keys in the `sync` block.
func WarnUnknownSyncKeys(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	syncRaw, ok := root["sync"]
	if !ok || syncRaw == nil {
		return nil, nil
	}

	syncMap, ok := asStringMap(syncRaw)
	if !ok {
		return nil, nil
	}

	var warnings []string
	warnings = appendUnknownMapKeys(warnings, "sync", syncMap, map[string]struct{}{
		"build":     {},
		"generated": {},
		"ignore":    {},
		"roots":     {},
	})

	if raw, ok := syncMap["build"]; ok {
		if build, ok := asStringMap(raw); ok {
			warnings = appendUnknownMapKeys(warnings, "sync.build", build, map[string]struct{}{
				"args":    {},
				"command": {},
				"cwd":     {},
				"env":     {},
			})
		}
	}

	if raw, ok := syncMap["roots"]; ok {
		if roots, ok := asStringMap(raw); ok {
			warnings = appendUnknownMapKeys(warnings, "sync.roots", roots, map[string]struct{}{
				"assets":    {},
				"layouts":   {},
				"snippets":  {},
				"templates": {},
			})
		}
	}

	return warnings, nil
}

func asStringMap(value any) (map[string]any, bool) {
	got, ok := value.(map[string]any)
	if ok {
		return got, true
	}

	gotNode, ok := value.(map[any]any)
	if !ok {
		return nil, false
	}

	out := make(map[string]any, len(gotNode))
	for key, value := range gotNode {
		keyStr, ok := key.(string)
		if !ok {
			return nil, false
		}
		out[keyStr] = value
	}
	return out, true
}

func appendUnknownMapKeys(warnings []string, scope string, got map[string]any, allowed map[string]struct{}) []string {
	for key := range got {
		if _, ok := allowed[key]; ok {
			continue
		}

		warnings = append(warnings, fmt.Sprintf("unknown %s key: %s", scope, key))
	}

	return warnings
}

// ParseRouteRule parses either "<glob>" or "<METHOD> <glob>" route rules.
func ParseRouteRule(raw string) (method string, pattern string) {
	rule := strings.TrimSpace(raw)
	if rule == "" {
		return "", ""
	}

	parts := strings.Fields(rule)
	if len(parts) == 1 {
		return "", parts[0]
	}

	maybeMethod := strings.ToUpper(parts[0])
	switch maybeMethod {
	case "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT":
		return maybeMethod, strings.Join(parts[1:], " ")
	default:
		return "", rule
	}
}

// WriteProjectConfig writes project config to nimbu.yml in the current directory.
func WriteProjectConfig(cfg ProjectConfig) error {
	return WriteProjectConfigTo(ProjectFileName, cfg)
}

// WriteProjectConfigTo writes project config to a specific path.
func WriteProjectConfigTo(path string, cfg ProjectConfig) error {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
