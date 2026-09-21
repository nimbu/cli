package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	ProjectFileName = "nimbu.yml"
	ProjectDirEnv   = "NIMBU_PROJECT_DIR"
)

// ProjectConfig holds project-specific configuration.
type ProjectConfig struct {
	Apps  []AppProjectConfig `json:"apps,omitempty" yaml:"apps,omitempty"`
	Site  string             `json:"site,omitempty" yaml:"site,omitempty"`
	Theme string             `json:"theme,omitempty" yaml:"theme,omitempty"`
	Dev   *DevConfig         `json:"dev,omitempty" yaml:"dev,omitempty"`
	Sync  *SyncConfig        `json:"sync,omitempty" yaml:"sync,omitempty"`
}

// AppProjectConfig configures one local cloud-code app mapping.
type AppProjectConfig struct {
	ID   string `json:"id,omitempty" yaml:"id,omitempty"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Dir  string `json:"dir,omitempty" yaml:"dir,omitempty"`
	Glob string `json:"glob,omitempty" yaml:"glob,omitempty"`
	Host string `json:"host,omitempty" yaml:"host,omitempty"`
	Site string `json:"site,omitempty" yaml:"site,omitempty"`
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

// ProjectLookupError reports a failed nimbu.yml lookup together with the
// directory where the upward search stopped. It unwraps to ErrNotFound so
// existing errors.Is checks keep working.
type ProjectLookupError struct {
	// StartDir is the directory the search started from.
	StartDir string
	// StoppedAt is the last directory that was inspected.
	StoppedAt string
	// EnvDriven reports whether the search started from NIMBU_PROJECT_DIR.
	EnvDriven bool
}

func (e *ProjectLookupError) Error() string {
	if e.EnvDriven {
		return fmt.Sprintf("%s not found from %s=%s (searched up to %s)", ProjectFileName, ProjectDirEnv, e.StartDir, e.StoppedAt)
	}
	return fmt.Sprintf("%s not found (searched up to %s)", ProjectFileName, e.StoppedAt)
}

func (e *ProjectLookupError) Unwrap() error { return ErrNotFound }

// DescribeProjectLookup renders the project file for user-facing error
// messages, e.g. `nimbu.yml (searched up to /home/me/site)`. Pass the error
// returned by a lookup; a nil or unrelated error falls back to a fresh search
// boundary computed from the current working directory.
func DescribeProjectLookup(err error) string {
	var lookupErr *ProjectLookupError
	if errors.As(err, &lookupErr) && lookupErr.StoppedAt != "" {
		return fmt.Sprintf("%s (searched up to %s)", ProjectFileName, lookupErr.StoppedAt)
	}
	if limit := ProjectSearchLimit(); limit != "" {
		return fmt.Sprintf("%s (searched up to %s)", ProjectFileName, limit)
	}
	return ProjectFileName
}

// ProjectSearchLimit returns the directory where an upward nimbu.yml search
// would stop: the enclosing git root when there is one, else the filesystem
// root. It returns "" when the start directory cannot be determined.
func ProjectSearchLimit() string {
	start := strings.TrimSpace(os.Getenv(ProjectDirEnv))
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return ""
		}
		start = cwd
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	_, stoppedAt := searchUpward(dir)
	return stoppedAt
}

// FindProjectFile locates nimbu.yml. Precedence:
//  1. Walk up from NIMBU_PROJECT_DIR, if set (error if nothing is found).
//  2. Walk up from the current working directory.
//  3. If still missing, try the git top-level directory as an extra candidate.
//
// Each walk stops at the filesystem root, or at the first directory holding a
// `.git` entry — that directory is still checked, nothing above it is.
func FindProjectFile() (string, error) {
	if raw := strings.TrimSpace(os.Getenv(ProjectDirEnv)); raw != "" {
		dir, err := filepath.Abs(raw)
		if err != nil {
			return "", fmt.Errorf("resolve %s=%q: %w", ProjectDirEnv, raw, err)
		}
		path, stoppedAt := searchUpward(dir)
		if path == "" {
			return "", &ProjectLookupError{StartDir: dir, StoppedAt: stoppedAt, EnvDriven: true}
		}
		noteProjectFileFromParent(path, dir)
		return path, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	path, stoppedAt := searchUpward(dir)
	if path != "" {
		noteProjectFileFromParent(path, dir)
		return path, nil
	}

	if top, ok := gitShowToplevel(dir); ok {
		if path, _ := searchUpward(top); path != "" {
			noteProjectFileFromParent(path, dir)
			return path, nil
		}
	}

	return "", &ProjectLookupError{StartDir: dir, StoppedAt: stoppedAt}
}

var projectFileNoteOnce sync.Once

// noteProjectFileFromParent logs, at most once per process, that the project
// file was picked up from a directory above the one we started in. It goes to
// slog.Info so it only surfaces with --verbose or --debug.
func noteProjectFileFromParent(path, startDir string) {
	dir := filepath.Dir(path)
	if dir == startDir {
		return
	}
	projectFileNoteOnce.Do(func() {
		slog.Info("using project config found in a parent directory",
			"path", path,
			"start_dir", startDir,
		)
	})
}

func gitShowToplevel(dir string) (string, bool) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	cmd.Env = withoutGitDirEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	top := strings.TrimSpace(string(out))
	if top == "" {
		return "", false
	}
	return top, true
}

func withoutGitDirEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_DIR=") ||
			strings.HasPrefix(kv, "GIT_WORK_TREE=") ||
			strings.HasPrefix(kv, "GIT_INDEX_FILE=") {
			continue
		}
		filtered = append(filtered, kv)
	}
	return filtered
}

// searchUpward walks from startDir towards the filesystem root looking for
// nimbu.yml. It returns the file path (empty when not found) and the last
// directory inspected, which is the boundary reported to users.
func searchUpward(startDir string) (path string, stoppedAt string) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, ProjectFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, dir
		}

		// The git root is inspected, but the search never climbs above it:
		// a repo boundary is the outer edge of a project.
		if isGitRoot(dir) {
			return "", dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root.
			return "", dir
		}
		dir = parent
	}
}

func isGitRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
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

// WarnUnknownAppsKeys returns warning strings for unknown keys in the `apps` block.
func WarnUnknownAppsKeys(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	appsRaw, ok := root["apps"]
	if !ok || appsRaw == nil {
		return nil, nil
	}

	apps, ok := appsRaw.([]any)
	if !ok {
		return nil, nil
	}

	var warnings []string
	for idx, raw := range apps {
		app, ok := asStringMap(raw)
		if !ok {
			continue
		}
		warnings = appendUnknownMapKeys(warnings, fmt.Sprintf("apps[%d]", idx), app, map[string]struct{}{
			"dir":  {},
			"glob": {},
			"host": {},
			"id":   {},
			"name": {},
			"site": {},
		})
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
