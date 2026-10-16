package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"time"

	"golang.org/x/term"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/devproxy"
	"github.com/nimbu/cli/internal/devserver"
	"github.com/nimbu/cli/internal/output"
)

// ServerCmd starts the local simulator proxy and child dev server.
type ServerCmd struct {
	Arg               []string      `help:"Child dev server arguments (repeatable)" name:"arg"`
	CMD               string        `help:"Override child dev server executable" name:"cmd"`
	CWD               string        `help:"Override child working directory"`
	Draft             []string      `help:"Request a draft preview token for each page and inject ?preview= on matching local URLs so you can browse the normal path. The simulator already honours ?preview=<token> when the browser URL carries it. Repeatable."`
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
	writer := output.WriterFromContext(ctx)
	presenter := newServerPresenter(ctx, c.EventsJSON)
	presenter.LimitBannerToTerminal(writer.ErrIsTTY())
	streams := newServerStreams(writer, presenter.Enabled())
	// Deferred first so it runs last: flush child lines after the child exited.
	defer streams.Close()
	presenter.SetOutput(streams.err)

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
		Diagnostics:       streams.err,
		EventsJSON:        c.EventsJSON,
		ExcludeRules:      runtimeCfg.RouteExclude,
		Host:              runtimeCfg.ProxyHost,
		IncludeRules:      runtimeCfg.RouteInclude,
		MaxBodyBytes:      int64(runtimeCfg.MaxBodyMB) << 20,
		Port:              runtimeCfg.ProxyPort,
		QuietRequests:     runtimeCfg.QuietRequests,
		RequestLog:        streams.out,
		Site:              site,
		TemplateRoot:      runtimeCfg.TemplateRoot,
		UseColor:          streams.requestLogColor,
		UserAgent:         "nimbu-go-cli",
		Watch:             runtimeCfg.Watch,
		WatchScanInterval: runtimeCfg.WatchScanInterval,
	}, client)
	if err != nil {
		return err
	}
	if err := enableServerDraftPreviews(ctx, client, c.Draft, proxy); err != nil {
		return err
	}

	if err := proxy.Start(); err != nil {
		return err
	}
	forced := false
	defer func() {
		if !forced {
			stopDevProxy(proxy)
		}
	}()

	proxyURL, err := url.Parse(proxy.URL())
	if err != nil {
		return fmt.Errorf("parse proxy URL: %w", err)
	}

	childEnv := buildServerChildEnv(runtimeCfg, proxyURL, proxy.URL(), site)
	applyServerChildColorEnv(childEnv, streams.childForceColor, os.Getenv)

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
	if interactiveShortcuts {
		shortcutListener, err = newServerShortcutListener()
		if err != nil {
			_, _ = fmt.Fprintf(streams.err, "warning: shortcut input unavailable: %s\n", err)
		} else {
			defer func() {
				if closeErr := shortcutListener.Close(); closeErr != nil {
					_, _ = fmt.Fprintf(streams.err, "warning: restore shortcut input: %s\n", closeErr)
				}
			}()
			// Panics on this goroutine unwind through the Close above; panics
			// on goroutines nimbu starts are covered by RestoreOnPanic.
			defer watchServerSuspend(shortcutListener)()
			summary.Shortcuts = shortcutLinks
			summary.Shortcuts.Active = true
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
		Stdout:   streams.childOut,
		Stderr:   streams.childErr,
	})

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, serverShutdownSignals()...)
	defer signal.Stop(sigCh)

	if err := child.Start(); err != nil {
		return err
	}
	defer func() {
		_ = child.Stop(5 * time.Second)
	}()

	// The goroutine owns its channel value: the loop below sets readyCh to nil
	// once consumed, which must not race with the send.
	readyCh := make(chan error, 1)
	go func(ready chan<- error) {
		defer shortcutListener.RestoreOnPanic()
		ready <- child.WaitReady(ctx, runtimeCfg.ReadyTimeout)
		close(ready)
	}(readyCh)

	shortcutReady := false
	var childExitCh <-chan error
	var shortcutEvents <-chan byte
	var shortcutErrors <-chan error
	if shortcutListener != nil {
		shortcutEvents = shortcutListener.Events()
		shortcutErrors = shortcutListener.Errors()
	}
	proxyErrors := proxy.Errors()

	stop := &serverStop{graceful: func() {
		_ = child.Stop(5 * time.Second)
		stopDevProxy(proxy)
	}}
	// requestStop handles Ctrl-C, SIGTERM/SIGHUP and 'q'. The first request
	// stops gracefully in the background; a second one forces the stop.
	requestStop := func(signalName string) error {
		if stop.Begin(shortcutListener) {
			presenter.PrintStopping(signalName)
			return nil
		}
		forced = true
		presenter.PrintForceStop()
		_ = child.Kill(2 * time.Second)
		_ = proxy.Close()
		if closeErr := shortcutListener.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(streams.err, "warning: restore shortcut input: %s\n", closeErr)
		}
		return errServerForceStopped
	}

	for {
		select {
		case sig := <-sigCh:
			if err := requestStop(sig.String()); err != nil {
				return err
			}
		case <-stop.Done():
			presenter.PrintGoodbye()
			return nil
		case err, ok := <-readyCh:
			readyCh = nil
			if !ok || stop.Stopping() {
				continue
			}
			if err != nil {
				_ = child.Stop(5 * time.Second)
				return err
			}
			shortcutReady = true
			childExitCh = child.ExitCh()
			if presenter.Enabled() {
				presenter.PrintReady(serverReadyTarget(summary))
			} else {
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
			if decision.Action == serverShortcutQuit {
				if err := requestStop(""); err != nil {
					return err
				}
				continue
			}
			if stop.Stopping() {
				continue
			}
			c.handleShortcut(decision, shortcutLinks, presenter, streams)
		case err, ok := <-shortcutErrors:
			if !ok {
				shortcutErrors = nil
				continue
			}
			if err != nil {
				presenter.PrintShortcutError(fmt.Sprintf("shortcut input disabled: %v", err))
			}
		case err, ok := <-proxyErrors:
			if !ok {
				proxyErrors = nil
				continue
			}
			if err == nil || stop.Stopping() {
				continue
			}
			_ = child.Stop(5 * time.Second)
			return fmt.Errorf("proxy server crashed: %w", err)
		case err, ok := <-childExitCh:
			childExitCh = nil
			if stop.Stopping() {
				continue
			}
			stopDevProxy(proxy)
			if ok && err != nil {
				return fmt.Errorf("child dev server exited: %w", err)
			}
			return fmt.Errorf("child dev server exited")
		}
	}
}

func (c *ServerCmd) handleShortcut(decision serverShortcutDecision, links serverShortcutLinks, presenter *serverPresenter, streams *serverStreams) {
	switch decision.Action {
	case serverShortcutNone, serverShortcutQuit:
		return
	case serverShortcutLogMarker:
		if err := writeServerShortcutLogMarker(streams.out); err != nil {
			presenter.PrintShortcutError(fmt.Sprintf("write log marker: %v", err))
		}
		return
	case serverShortcutHelp:
		presenter.PrintShortcutHelp(links)
		return
	}
	if decision.Pending {
		presenter.PrintShortcutPending()
		return
	}
	label, target, ok := links.target(decision.Action)
	if !ok {
		return
	}
	if err := openServerBrowserURL(target, nil); err != nil {
		presenter.PrintShortcutError(fmt.Sprintf("open %s: %v", label, err))
	}
}

// serverReadyTarget is the URL to browse once everything answers: the child
// dev server when it has a readiness URL, otherwise the proxy.
func serverReadyTarget(summary serverSummary) string {
	if target := displayServerURL(summary.ReadyURL); target != "" {
		return target
	}
	return displayServerURL(summary.ProxyURL)
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
