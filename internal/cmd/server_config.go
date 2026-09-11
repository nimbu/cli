package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/config"
)

const (
	defaultProxyHost         = "127.0.0.1"
	defaultProxyPort         = 4568
	defaultWatchScanInterval = 3 * time.Second
	defaultMaxBodyMB         = 64
	defaultReadyTimeout      = 60 * time.Second
)

type serverRuntimeConfig struct {
	ChildArgs         []string
	ChildCommand      string
	ChildCWD          string
	ChildEnv          map[string]string
	MaxBodyMB         int
	DevToken          string
	ProjectRoot       string
	ProxyHost         string
	ProxyPort         int
	QuietRequests     bool
	ReadyTimeout      time.Duration
	ReadyURL          string
	RouteExclude      []string
	RouteInclude      []string
	TemplateRoot      string
	Watch             bool
	WatchScanInterval time.Duration
}

func (c *ServerCmd) resolveRuntimeConfig() (serverRuntimeConfig, []string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return serverRuntimeConfig{}, nil, err
	}

	projectRoot := cwd
	var projectCfg config.ProjectConfig
	var warnings []string

	projectFile, err := config.FindProjectFile()
	if err == nil {
		projectRoot = filepath.Dir(projectFile)
		projectCfg, err = config.ReadProjectConfigFrom(projectFile)
		if err != nil {
			return serverRuntimeConfig{}, nil, fmt.Errorf("read project config: %w", err)
		}
		if keyWarnings, warnErr := config.WarnUnknownDevKeys(projectFile); warnErr == nil {
			warnings = append(warnings, keyWarnings...)
		}
	} else if !errors.Is(err, config.ErrNotFound) {
		return serverRuntimeConfig{}, nil, err
	}

	cfg := serverRuntimeConfig{
		ChildCWD:          projectRoot,
		ChildEnv:          map[string]string{},
		MaxBodyMB:         defaultMaxBodyMB,
		ProjectRoot:       projectRoot,
		ProxyHost:         defaultProxyHost,
		ProxyPort:         defaultProxyPort,
		ReadyTimeout:      defaultReadyTimeout,
		TemplateRoot:      projectRoot,
		Watch:             true,
		WatchScanInterval: defaultWatchScanInterval,
	}
	var watchScanIntervalRaw string

	if projectCfg.Dev != nil {
		dev := projectCfg.Dev
		if dev.Proxy.Host != "" {
			cfg.ProxyHost = dev.Proxy.Host
		}
		if dev.Proxy.Port != 0 {
			if dev.Proxy.Port < 0 || dev.Proxy.Port > 65535 {
				return serverRuntimeConfig{}, nil, fmt.Errorf("invalid dev.proxy.port: must be between 1 and 65535")
			}
			cfg.ProxyPort = dev.Proxy.Port
		}
		if dev.Proxy.TemplateRoot != "" {
			cfg.TemplateRoot = resolveFromProjectRoot(projectRoot, dev.Proxy.TemplateRoot)
		}
		if dev.Proxy.MaxBodyMB != 0 {
			if dev.Proxy.MaxBodyMB < 0 {
				return serverRuntimeConfig{}, nil, fmt.Errorf("invalid dev.proxy.max_body_mb: must be positive")
			}
			cfg.MaxBodyMB = dev.Proxy.MaxBodyMB
		}
		if dev.Proxy.Watch != nil {
			cfg.Watch = *dev.Proxy.Watch
		}
		if dev.Proxy.WatchScanInterval != "" {
			watchScanIntervalRaw = dev.Proxy.WatchScanInterval
		}

		cfg.RouteInclude = append(cfg.RouteInclude, dev.Routes.Include...)
		cfg.RouteExclude = append(cfg.RouteExclude, dev.Routes.Exclude...)

		if dev.Server.Command != "" {
			cfg.ChildCommand = dev.Server.Command
		}
		if len(dev.Server.Args) > 0 {
			cfg.ChildArgs = append([]string{}, dev.Server.Args...)
		}
		if dev.Server.CWD != "" {
			cfg.ChildCWD = resolveFromProjectRoot(projectRoot, dev.Server.CWD)
		}
		if dev.Server.ReadyURL != "" {
			cfg.ReadyURL = dev.Server.ReadyURL
		}
		for key, value := range dev.Server.Env {
			cfg.ChildEnv[key] = value
		}
	}

	if c.ProxyHost != "" {
		cfg.ProxyHost = c.ProxyHost
	}
	if c.ProxyPort != 0 {
		if c.ProxyPort < 0 || c.ProxyPort > 65535 {
			return serverRuntimeConfig{}, warnings, fmt.Errorf("proxy port must be between 1 and 65535")
		}
		cfg.ProxyPort = c.ProxyPort
	}
	if c.MaxBodyMB != 0 {
		if c.MaxBodyMB < 0 {
			return serverRuntimeConfig{}, warnings, fmt.Errorf("max body size must be positive")
		}
		cfg.MaxBodyMB = c.MaxBodyMB
	}
	if c.TemplateRoot != "" {
		cfg.TemplateRoot = resolveFromProjectRoot(projectRoot, c.TemplateRoot)
	}
	if c.CWD != "" {
		cfg.ChildCWD = resolveFromProjectRoot(projectRoot, c.CWD)
	}
	if c.CMD != "" {
		cfg.ChildCommand = c.CMD
	}
	if len(c.Arg) > 0 {
		cfg.ChildArgs = append([]string{}, c.Arg...)
	}
	if c.ReadyURL != "" {
		cfg.ReadyURL = c.ReadyURL
	}
	if c.NoWatch {
		cfg.Watch = false
	}
	if c.WatchScanInterval < 0 {
		return serverRuntimeConfig{}, warnings, fmt.Errorf("watch scan interval must be positive")
	}
	if c.WatchScanInterval > 0 {
		cfg.WatchScanInterval = c.WatchScanInterval
		watchScanIntervalRaw = ""
	}
	if watchScanIntervalRaw != "" {
		d, parseErr := time.ParseDuration(watchScanIntervalRaw)
		if parseErr != nil {
			return serverRuntimeConfig{}, warnings, fmt.Errorf("invalid dev.proxy.watch_scan_interval: %w", parseErr)
		}
		cfg.WatchScanInterval = d
	}
	if c.ReadyTimeout < 0 {
		return serverRuntimeConfig{}, warnings, fmt.Errorf("ready timeout must be positive")
	}
	if c.ReadyTimeout > 0 {
		cfg.ReadyTimeout = c.ReadyTimeout
	}
	cfg.QuietRequests = c.QuietRequests

	if cfg.ChildCommand == "" {
		return serverRuntimeConfig{}, warnings, fmt.Errorf("child dev server command required; set dev.server.command in nimbu.yml or pass --cmd")
	}
	if cfg.ProxyPort <= 0 || cfg.ProxyPort > 65535 {
		return serverRuntimeConfig{}, warnings, fmt.Errorf("proxy port must be between 1 and 65535")
	}
	if cfg.MaxBodyMB <= 0 {
		return serverRuntimeConfig{}, warnings, fmt.Errorf("max body size must be positive")
	}
	if cfg.ReadyURL != "" {
		if _, parseErr := url.ParseRequestURI(cfg.ReadyURL); parseErr != nil {
			return serverRuntimeConfig{}, warnings, fmt.Errorf("invalid ready URL: %w", parseErr)
		}
	}
	token, err := generateDevProxyToken()
	if err != nil {
		return serverRuntimeConfig{}, warnings, fmt.Errorf("generate dev proxy token: %w", err)
	}
	cfg.DevToken = token

	cfg.RouteInclude = normalizeRules(cfg.RouteInclude)
	cfg.RouteExclude = normalizeRules(cfg.RouteExclude)
	return cfg, warnings, nil
}

func generateDevProxyToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func resolveFromProjectRoot(projectRoot string, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return projectRoot
	}
	if filepath.IsAbs(trimmed) {
		return trimmed
	}
	return filepath.Join(projectRoot, trimmed)
}

func normalizeRules(rules []string) []string {
	out := make([]string, 0, len(rules))
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		out = append(out, rule)
	}
	return out
}
