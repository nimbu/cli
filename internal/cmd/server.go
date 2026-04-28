package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/devproxy"
	"github.com/nimbu/cli/internal/devserver"
)

const (
	defaultProxyHost         = "127.0.0.1"
	defaultProxyPort         = 4568
	defaultWatchScanInterval = 3 * time.Second
	defaultMaxBodyMB         = 64
	defaultReadyTimeout      = 60 * time.Second
)

// ServerCmd starts the local simulator proxy and child dev server.
type ServerCmd struct {
	Arg               []string      `help:"Child dev server arguments (repeatable)" name:"arg"`
	CMD               string        `help:"Override child dev server executable" name:"cmd"`
	CWD               string        `help:"Override child working directory"`
	EventsJSON        bool          `help:"Emit structured runtime events as JSON lines"`
	MaxBodyMB         int           `help:"Max request body size in MB for simulator proxy" default:"0"`
	NoWatch           bool          `help:"Disable filesystem watcher invalidation"`
	ProxyHost         string        `help:"Proxy host" default:""`
	ProxyPort         int           `help:"Proxy port" default:"0"`
	QuietRequests     bool          `help:"Disable per-request proxy log lines"`
	ReadyTimeout      time.Duration `help:"Child readiness timeout" default:"0s"`
	ReadyURL          string        `help:"Override child readiness URL"`
	TemplateRoot      string        `help:"Override template root directory"`
	WatchScanInterval time.Duration `help:"Fallback template scan interval" default:"0s"`
}

func (c *ServerCmd) Run(ctx context.Context, flags *RootFlags) error {
	runtimeCfg, warnings, err := c.resolveRuntimeConfig()
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	presenter := newServerPresenter(ctx, c.EventsJSON)

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	if err := c.validateLogin(ctx, client); err != nil {
		return err
	}
	presenter.PrintBanner()
	siteSubdomain := lookupSiteSubdomain(ctx, client, site)

	proxy, err := devproxy.New(devproxy.Config{
		APIURL:            client.BaseURL,
		Debug:             flags.Debug,
		DevToken:          runtimeCfg.DevToken,
		EventsJSON:        c.EventsJSON,
		ExcludeRules:      runtimeCfg.RouteExclude,
		Host:              runtimeCfg.ProxyHost,
		IncludeRules:      runtimeCfg.RouteInclude,
		MaxBodyBytes:      int64(runtimeCfg.MaxBodyMB) << 20,
		Port:              runtimeCfg.ProxyPort,
		QuietRequests:     runtimeCfg.QuietRequests,
		Site:              site,
		TemplateRoot:      runtimeCfg.TemplateRoot,
		UseColor:          presenter.UseColor(),
		UserAgent:         "nimbu-go-cli",
		Watch:             runtimeCfg.Watch,
		WatchScanInterval: runtimeCfg.WatchScanInterval,
	}, client)
	if err != nil {
		return err
	}

	if err := proxy.Start(); err != nil {
		return err
	}
	defer stopDevProxy(proxy)

	proxyURL, err := url.Parse(proxy.URL())
	if err != nil {
		return fmt.Errorf("parse proxy URL: %w", err)
	}

	childEnv := buildServerChildEnv(runtimeCfg, proxyURL, proxy.URL(), site)

	summary := serverSummary{
		APIHost:      serverAPIHost(client.BaseURL),
		ChildCommand: formatServerChildCommand(runtimeCfg.ChildCommand, runtimeCfg.ChildArgs),
		ProxyURL:     proxy.URL(),
		ReadyURL:     runtimeCfg.ReadyURL,
		SiteHost:     siteHostFromAPI(client.BaseURL, siteSubdomain),
		SiteLabel:    compactSiteLabel(siteSubdomain),
	}
	if cwd := displayPathFromRoot(runtimeCfg.ProjectRoot, runtimeCfg.ChildCWD); cwd != "." {
		summary.ChildCWD = cwd
	}

	interactiveShortcuts := presenter.Enabled() && term.IsTerminal(int(os.Stdin.Fd()))
	shortcutLinks := serverShortcutLinksFromSummary(summary)
	var shortcutListener *serverShortcutListener
	if interactiveShortcuts && shortcutLinks.Hint() != "" {
		shortcutListener, err = newServerShortcutListener()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: shortcut input unavailable: %s\n", err)
		} else {
			defer func() {
				if closeErr := shortcutListener.Close(); closeErr != nil {
					_, _ = fmt.Fprintf(os.Stderr, "warning: restore shortcut input: %s\n", closeErr)
				}
			}()
			summary.Shortcuts = shortcutLinks
		}
	}

	if presenter.Enabled() {
		presenter.PrintSummary(summary)
	}

	child := devserver.NewProcess(devserver.ChildConfig{
		Args:     runtimeCfg.ChildArgs,
		Command:  runtimeCfg.ChildCommand,
		CWD:      runtimeCfg.ChildCWD,
		Env:      childEnv,
		ReadyURL: runtimeCfg.ReadyURL,
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, serverShutdownSignals()...)
	defer signal.Stop(sigCh)

	if err := child.Start(); err != nil {
		return err
	}
	defer func() {
		_ = child.Stop(5 * time.Second)
	}()

	readyCh := make(chan error, 1)
	go func() {
		readyCh <- child.WaitReady(ctx, runtimeCfg.ReadyTimeout)
		close(readyCh)
	}()

	shortcutReady := false
	var childExitCh <-chan error
	var shortcutEvents <-chan byte
	var shortcutErrors <-chan error
	if shortcutListener != nil {
		shortcutEvents = shortcutListener.Events()
		shortcutErrors = shortcutListener.Errors()
	}

	shutdown := func(signalName string) error {
		if presenter.Enabled() {
			presenter.PrintShutdownNotice()
		} else if signalName != "" {
			_, _ = fmt.Fprintf(os.Stderr, "received signal: %s\n", signalName)
		}
		_ = child.Stop(5 * time.Second)
		stopDevProxy(proxy)
		presenter.PrintGoodbye()
		return nil
	}

	for {
		select {
		case sig := <-sigCh:
			return shutdown(sig.String())
		case err, ok := <-readyCh:
			if !ok {
				readyCh = nil
				continue
			}
			readyCh = nil
			if err != nil {
				_ = child.Stop(5 * time.Second)
				return err
			}
			shortcutReady = true
			childExitCh = child.ExitCh()
			if !presenter.Enabled() {
				_, _ = fmt.Fprintf(os.Stderr, "proxy ready: %s\n", proxy.URL())
				if runtimeCfg.ReadyURL != "" {
					_, _ = fmt.Fprintf(os.Stderr, "dev server ready: %s\n", runtimeCfg.ReadyURL)
				}
			}
		case key, ok := <-shortcutEvents:
			if !ok {
				shortcutEvents = nil
				continue
			}
			decision := decideServerShortcut(key, shortcutReady, shortcutLinks)
			if decision.Action == serverShortcutNone {
				continue
			}
			if decision.Action == serverShortcutLogMarker {
				if err := writeServerShortcutLogMarker(os.Stdout); err != nil {
					presenter.PrintShortcutError(fmt.Sprintf("write log marker: %v", err))
				}
				continue
			}
			if decision.Pending {
				presenter.PrintShortcutPending()
				continue
			}
			label, target, ok := shortcutLinks.target(decision.Action)
			if !ok {
				continue
			}
			if err := openServerBrowserURL(target, nil); err != nil {
				presenter.PrintShortcutError(fmt.Sprintf("open %s: %v", label, err))
			}
		case err, ok := <-shortcutErrors:
			if !ok {
				shortcutErrors = nil
				continue
			}
			if err != nil {
				presenter.PrintShortcutError(fmt.Sprintf("shortcut input disabled: %v", err))
			}
		case err, ok := <-proxy.Errors():
			if !ok || err == nil {
				continue
			}
			_ = child.Stop(5 * time.Second)
			return fmt.Errorf("proxy server crashed: %w", err)
		case err, ok := <-childExitCh:
			if !ok {
				stopDevProxy(proxy)
				return fmt.Errorf("child dev server exited")
			}
			stopDevProxy(proxy)
			if err != nil {
				return fmt.Errorf("child dev server exited: %w", err)
			}
			return fmt.Errorf("child dev server exited")
		}
	}
}

func stopDevProxy(proxy *devproxy.Server) {
	if proxy == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = proxy.Stop(shutdownCtx)
	cancel()
}

func buildServerChildEnv(runtimeCfg serverRuntimeConfig, proxyURL *url.URL, proxyURLString string, site string) map[string]string {
	childEnv := make(map[string]string, len(runtimeCfg.ChildEnv)+5)
	for key, value := range runtimeCfg.ChildEnv {
		childEnv[key] = value
	}
	childEnv["NIMBU_PROXY_URL"] = proxyURLString
	childEnv["NIMBU_PROXY_HOST"] = proxyURL.Hostname()
	childEnv["NIMBU_PROXY_PORT"] = proxyURL.Port()
	childEnv["NIMBU_DEV_PROXY_TOKEN"] = runtimeCfg.DevToken
	if site != "" {
		if _, exists := childEnv["NIMBU_SITE"]; !exists {
			childEnv["NIMBU_SITE"] = site
		}
	}
	return childEnv
}

func (c *ServerCmd) validateLogin(ctx context.Context, client *api.Client) error {
	var user map[string]any
	// use raw Request to avoid scope-specific endpoints; /user is already used in toolbelt.
	if err := client.Get(ctx, "/user", &user); err != nil {
		return fmt.Errorf("authentication check failed: %w", err)
	}
	return nil
}

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
