//go:build !windows

package cmd

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestServerShutdownSignalsIncludeSIGHUP(t *testing.T) {
	for _, sig := range serverShutdownSignals() {
		if sig == syscall.SIGHUP {
			return
		}
	}
	t.Fatalf("server shutdown signals = %v, want SIGHUP", serverShutdownSignals())
}

func TestServerRunStopsChildOnSIGHUP(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{"id":"user-1"}`))
		case "/sites/demo":
			_, _ = w.Write([]byte(`{"id":"site-1","subdomain":"demo"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}

	readyAddr := reserveTCPAddr(t)
	readyURL := "http://" + readyAddr + "/ready"
	proxyPort := reserveTCPPort(t)
	stoppedPath := filepath.Join(root, "child-stopped")
	pidPath := filepath.Join(root, "child-pid")

	t.Setenv("NIMBU_TOKEN", "test-token")
	t.Setenv("NIMBU_SERVER_SIGNAL_HELPER", "1")
	t.Setenv("NIMBU_SERVER_SIGNAL_READY_ADDR", readyAddr)
	t.Setenv("NIMBU_SERVER_SIGNAL_STOPPED_PATH", stoppedPath)
	t.Setenv("NIMBU_SERVER_SIGNAL_PID_PATH", pidPath)

	flags := &RootFlags{
		APIURL:  apiServer.URL,
		Site:    "demo",
		Timeout: 2 * time.Second,
	}
	ctx := newServerSignalTestContext(flags)

	cmd := &ServerCmd{
		Arg:          []string{"-test.run=^TestServerSignalChildHelperProcess$"},
		CMD:          os.Args[0],
		NoWatch:      true,
		ProxyHost:    "127.0.0.1",
		ProxyPort:    proxyPort,
		ReadyTimeout: 5 * time.Second,
		ReadyURL:     readyURL,
		TemplateRoot: root,
	}

	sighupGuard := make(chan os.Signal, 1)
	signal.Notify(sighupGuard, syscall.SIGHUP)
	defer signal.Stop(sighupGuard)

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Run(ctx, flags)
	}()
	t.Cleanup(func() {
		if _, err := os.Stat(stoppedPath); err == nil {
			return
		}
		cleanupServerSignalChild(pidPath)
	})

	waitForServerSignalReady(t, readyURL, errCh)

	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("send SIGHUP: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not shut down after SIGHUP")
	}

	if !waitForServerSignalFile(stoppedPath, 5*time.Second) {
		t.Fatal("child did not record graceful shutdown")
	}
}

func TestServerSignalChildHelperProcess(t *testing.T) {
	if os.Getenv("NIMBU_SERVER_SIGNAL_HELPER") != "1" {
		return
	}

	addr := os.Getenv("NIMBU_SERVER_SIGNAL_READY_ADDR")
	if addr == "" {
		t.Fatal("missing NIMBU_SERVER_SIGNAL_READY_ADDR")
	}
	if pidPath := os.Getenv("NIMBU_SERVER_SIGNAL_PID_PATH"); pidPath != "" {
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
			t.Fatalf("write pid file: %v", err)
		}
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen on ready addr: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second}

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- srv.Serve(ln)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if os.Getenv("NIMBU_SERVER_SIGNAL_IGNORE_INT") == "1" {
		// Simulate a dev server that hangs on shutdown: note the interrupt,
		// keep serving, and wait to be killed.
		<-sigCh
		if path := os.Getenv("NIMBU_SERVER_SIGNAL_INTERRUPTED_PATH"); path != "" {
			_ = os.WriteFile(path, []byte("interrupted"), 0o644)
		}
		time.Sleep(20 * time.Second)
		t.Fatal("helper was not killed")
	}

	select {
	case <-sigCh:
		if stoppedPath := os.Getenv("NIMBU_SERVER_SIGNAL_STOPPED_PATH"); stoppedPath != "" {
			if err := os.WriteFile(stoppedPath, []byte("stopped"), 0o644); err != nil {
				t.Fatalf("write stopped file: %v", err)
			}
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	case err := <-serveErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("serve ready endpoint: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("helper timed out waiting for shutdown signal")
	}
}

func newServerSignalTestContext(flags *RootFlags) context.Context {
	var stdout, stderr bytes.Buffer
	mode := output.Mode{}
	ctx := context.Background()
	ctx = output.WithMode(ctx, mode)
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   &stdout,
		Err:   &stderr,
		Mode:  mode,
		Color: "never",
		NoTTY: true,
	})
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})
	return ctx
}

func reserveTCPAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve tcp addr: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()

	addr := reserveTCPAddr(t)
	_, portRaw, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return port
}

func waitForServerSignalReady(t *testing.T, readyURL string, errCh <-chan error) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("server exited before child was ready: %v", err)
		default:
		}

		resp, err := client.Get(readyURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				time.Sleep(200 * time.Millisecond)
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("child readiness endpoint did not become ready at %s", readyURL)
}

func waitForServerSignalFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func cleanupServerSignalChild(pidPath string) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(string(bytes.TrimSpace(data)))
	if err != nil || pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func TestServerRunSecondInterruptForcesStop(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{"id":"user-1"}`))
		case "/sites/demo":
			_, _ = w.Write([]byte(`{"id":"site-1","subdomain":"demo"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	readyAddr := reserveTCPAddr(t)
	readyURL := "http://" + readyAddr + "/ready"
	pidPath := filepath.Join(root, "child-pid")
	interruptedPath := filepath.Join(root, "child-interrupted")

	t.Setenv("NIMBU_TOKEN", "test-token")
	t.Setenv("NIMBU_SERVER_SIGNAL_HELPER", "1")
	t.Setenv("NIMBU_SERVER_SIGNAL_IGNORE_INT", "1")
	t.Setenv("NIMBU_SERVER_SIGNAL_READY_ADDR", readyAddr)
	t.Setenv("NIMBU_SERVER_SIGNAL_PID_PATH", pidPath)
	t.Setenv("NIMBU_SERVER_SIGNAL_INTERRUPTED_PATH", interruptedPath)

	flags := &RootFlags{APIURL: apiServer.URL, Site: "demo", Timeout: 2 * time.Second}
	// Run under the real root handler, as Execute does: the server must own
	// both presses, so the root neither cancels ctx on the first (racing the
	// readiness wait) nor force-exits on the second (orphaning the child and
	// leaving shortcut input mode on).
	ctx, rootExits := withTestRootInterrupt(t, newServerSignalTestContext(flags))
	cmd := &ServerCmd{
		Arg:          []string{"-test.run=^TestServerSignalChildHelperProcess$"},
		CMD:          os.Args[0],
		NoWatch:      true,
		ProxyHost:    "127.0.0.1",
		ProxyPort:    reserveTCPPort(t),
		ReadyTimeout: 5 * time.Second,
		ReadyURL:     readyURL,
		TemplateRoot: root,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Run(ctx, flags)
	}()
	t.Cleanup(func() { cleanupServerSignalChild(pidPath) })

	waitForServerSignalReady(t, readyURL, errCh)

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send first SIGINT: %v", err)
	}
	if !waitForServerSignalFile(interruptedPath, 5*time.Second) {
		t.Fatal("child never saw the graceful interrupt")
	}
	select {
	case err := <-errCh:
		t.Fatalf("run returned during the graceful stop: %v", err)
	default:
	}
	if ctx.Err() != nil {
		t.Fatal("the root canceled ctx on a signal the server claimed")
	}

	forcedAt := time.Now()
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send second SIGINT: %v", err)
	}

	select {
	case err := <-errCh:
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 130 {
			t.Fatalf("run error = %v, want exit 130", err)
		}
		// The graceful path would wait out the 5s child grace period.
		if elapsed := time.Since(forcedAt); elapsed > 3*time.Second {
			t.Fatalf("forced stop took %s", elapsed)
		}
		assertNoForceExit(t, rootExits)
	case <-time.After(4 * time.Second):
		t.Fatal("second SIGINT did not force the stop")
	}

	pid := readServerSignalPID(t, pidPath)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child %d survived the forced stop", pid)
}

func readServerSignalPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	pid, err := strconv.Atoi(string(bytes.TrimSpace(data)))
	if err != nil || pid <= 0 {
		t.Fatalf("bad pid %q: %v", data, err)
	}
	return pid
}

type fakeServerSuspender struct{ calls []string }

func (f *fakeServerSuspender) Suspend() error { f.calls = append(f.calls, "suspend"); return nil }
func (f *fakeServerSuspender) Resume() error  { f.calls = append(f.calls, "resume"); return nil }

func TestHandleServerSuspendSignal(t *testing.T) {
	term := &fakeServerSuspender{}
	stopSelf := func() { term.calls = append(term.calls, "stop") }

	handleServerSuspendSignal(syscall.SIGTSTP, term, stopSelf)
	handleServerSuspendSignal(syscall.SIGCONT, term, stopSelf)

	if got, want := strings.Join(term.calls, ","), "suspend,stop,resume"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}
}

// A failure (here: readiness timeout) stops the child in the background too,
// so Ctrl-C still forces the stop instead of waiting out the grace period of
// a child that ignores the interrupt. Run keeps returning the failure.
func TestServerRunInterruptForcesStopAfterReadinessFailure(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{"id":"user-1"}`))
		case "/sites/demo":
			_, _ = w.Write([]byte(`{"id":"site-1","subdomain":"demo"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	pidPath := filepath.Join(root, "child-pid")
	interruptedPath := filepath.Join(root, "child-interrupted")

	t.Setenv("NIMBU_TOKEN", "test-token")
	t.Setenv("NIMBU_SERVER_SIGNAL_HELPER", "1")
	t.Setenv("NIMBU_SERVER_SIGNAL_IGNORE_INT", "1")
	t.Setenv("NIMBU_SERVER_SIGNAL_READY_ADDR", reserveTCPAddr(t))
	t.Setenv("NIMBU_SERVER_SIGNAL_PID_PATH", pidPath)
	t.Setenv("NIMBU_SERVER_SIGNAL_INTERRUPTED_PATH", interruptedPath)

	flags := &RootFlags{APIURL: apiServer.URL, Site: "demo", Timeout: 2 * time.Second}
	// Run under the real root handler, as Execute does: the server must own
	// both presses, so the root neither cancels ctx on the first (racing the
	// readiness wait) nor force-exits on the second (orphaning the child and
	// leaving shortcut input mode on).
	ctx, rootExits := withTestRootInterrupt(t, newServerSignalTestContext(flags))
	cmd := &ServerCmd{
		Arg:       []string{"-test.run=^TestServerSignalChildHelperProcess$"},
		CMD:       os.Args[0],
		NoWatch:   true,
		ProxyHost: "127.0.0.1",
		ProxyPort: reserveTCPPort(t),
		// Nothing listens here, so readiness times out.
		ReadyTimeout: 500 * time.Millisecond,
		ReadyURL:     "http://" + reserveTCPAddr(t) + "/ready",
		TemplateRoot: root,
	}

	guard := make(chan os.Signal, 4)
	signal.Notify(guard, os.Interrupt)
	defer signal.Stop(guard)

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Run(ctx, flags)
	}()
	t.Cleanup(func() { cleanupServerSignalChild(pidPath) })

	// The child sees the graceful interrupt once the failure cleanup began.
	if !waitForServerSignalFile(interruptedPath, 5*time.Second) {
		t.Fatal("failure cleanup never interrupted the child")
	}
	forcedAt := time.Now()
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "readiness timeout") {
			t.Fatalf("run error = %v, want the readiness timeout", err)
		}
		if elapsed := time.Since(forcedAt); elapsed > 3*time.Second {
			t.Fatalf("forced stop took %s", elapsed)
		}
		assertNoForceExit(t, rootExits)
	case <-time.After(4 * time.Second):
		t.Fatal("SIGINT during failure cleanup did not force the stop")
	}
}
