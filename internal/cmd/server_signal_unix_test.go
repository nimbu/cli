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
