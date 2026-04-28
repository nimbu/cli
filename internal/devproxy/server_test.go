package devproxy

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/api"
)

func TestIsWebSocketUpgradeRequiresConnectionUpgrade(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "keep-alive")

	if isWebSocketUpgrade(req) {
		t.Fatal("expected false when connection header misses upgrade token")
	}

	req.Header.Set("Connection", "Upgrade")
	if !isWebSocketUpgrade(req) {
		t.Fatal("expected websocket upgrade detection to pass")
	}
}

func TestLoggingMiddlewareDisablesANSIForEventsJSON(t *testing.T) {
	server := &Server{config: Config{UseColor: true, EventsJSON: true}}
	handler := server.loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	output := captureStdout(t, func() {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/ons-werk", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	})

	if strings.Contains(output, "\x1b[") {
		t.Fatalf("expected plain request log, got ANSI: %q", output)
	}
	if strings.HasPrefix(output, "\n") {
		t.Fatalf("did not expect spacer line for events-json output, got: %q", output)
	}
	if !strings.Contains(output, "GET /ons-werk (200)") {
		t.Fatalf("unexpected request log output: %q", output)
	}
}

func TestLoggingMiddlewarePrintsSingleSpacerBeforeFirstRequest(t *testing.T) {
	server := &Server{config: Config{UseColor: false}}
	handler := server.loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	output := captureStdout(t, func() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.com/first", nil))

		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.com/second", nil))
	})

	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected spacer plus two request logs, got %d lines: %q", len(lines), output)
	}
	if lines[0] != "" {
		t.Fatalf("expected first line to be blank spacer, got: %q", lines[0])
	}
	if !strings.Contains(lines[1], "GET /first (200)") {
		t.Fatalf("unexpected first request log: %q", lines[1])
	}
	if !strings.Contains(lines[2], "GET /second (200)") {
		t.Fatalf("unexpected second request log: %q", lines[2])
	}
}

func TestTemplateOverlayEndpointRequiresToken(t *testing.T) {
	server := &Server{
		cache:  NewTemplateCache(t.TempDir(), false, time.Second, &Logger{}),
		config: Config{DevToken: "secret"},
	}

	req := httptest.NewRequest(http.MethodPut, "http://example.com/__nimbu/dev/templates/overlays", strings.NewReader(`{"templates":[]}`))
	rec := httptest.NewRecorder()

	server.handleTemplateOverlays(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestTemplateOverlayEndpointRejectsWrongToken(t *testing.T) {
	server := &Server{
		cache:  NewTemplateCache(t.TempDir(), false, time.Second, &Logger{}),
		config: Config{DevToken: "secret"},
	}

	req := httptest.NewRequest(http.MethodPut, "http://example.com/__nimbu/dev/templates/overlays", strings.NewReader(`{"templates":[]}`))
	req.Header.Set("X-Nimbu-Dev-Token", "wrong")
	rec := httptest.NewRecorder()

	server.handleTemplateOverlays(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestTemplateOverlayEndpointUpdatesCache(t *testing.T) {
	cache := NewTemplateCache(t.TempDir(), false, time.Second, &Logger{})
	server := &Server{
		cache:  cache,
		config: Config{DevToken: "secret"},
	}

	body := `{"templates":[{"type":"snippets","path":"bundle_app.liquid","content":"virtual bundle"}]}`
	req := httptest.NewRequest(http.MethodPut, "http://example.com/__nimbu/dev/templates/overlays", strings.NewReader(body))
	req.Header.Set("X-Nimbu-Dev-Token", "secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleTemplateOverlays(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := cache.Status().OverlayCount; got != 1 {
		t.Fatalf("overlay count = %d, want 1", got)
	}
	if !strings.Contains(rec.Body.String(), `"count":1`) {
		t.Fatalf("response should include count, got %s", rec.Body.String())
	}
}

func TestTemplateOverlayEndpointDeleteClearsCache(t *testing.T) {
	cache := NewTemplateCache(t.TempDir(), false, time.Second, &Logger{})
	if err := cache.SetOverlays([]TemplateOverlay{
		{Type: "snippets", Path: "bundle_app.liquid", Content: "virtual bundle"},
	}); err != nil {
		t.Fatalf("seed overlays: %v", err)
	}
	server := &Server{
		cache:  cache,
		config: Config{DevToken: "secret"},
	}

	req := httptest.NewRequest(http.MethodDelete, "http://example.com/__nimbu/dev/templates/overlays", nil)
	req.Header.Set("X-Nimbu-Dev-Token", "secret")
	rec := httptest.NewRecorder()

	server.handleTemplateOverlays(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := cache.Status().OverlayCount; got != 0 {
		t.Fatalf("overlay count = %d, want 0", got)
	}
}

func TestTemplateOverlayEndpointRejectsInvalidOverlay(t *testing.T) {
	cache := NewTemplateCache(t.TempDir(), false, time.Second, &Logger{})
	server := &Server{
		cache:  cache,
		config: Config{DevToken: "secret"},
	}

	body := `{"templates":[{"type":"snippets","path":"../bundle_app.liquid","content":"bad"}]}`
	req := httptest.NewRequest(http.MethodPut, "http://example.com/__nimbu/dev/templates/overlays", strings.NewReader(body))
	req.Header.Set("X-Nimbu-Dev-Token", "secret")
	rec := httptest.NewRecorder()

	server.handleTemplateOverlays(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestTemplateOverlayEndpointRejectsTrailingJSON(t *testing.T) {
	cache := NewTemplateCache(t.TempDir(), false, time.Second, &Logger{})
	server := &Server{
		cache:  cache,
		config: Config{DevToken: "secret"},
	}

	req := httptest.NewRequest(http.MethodPut, "http://example.com/__nimbu/dev/templates/overlays", strings.NewReader(`{"templates":[]} {}`))
	req.Header.Set("X-Nimbu-Dev-Token", "secret")
	rec := httptest.NewRecorder()

	server.handleTemplateOverlays(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestTemplateOverlayEndpointRejectsUnsupportedMethods(t *testing.T) {
	server := &Server{
		cache:  NewTemplateCache(t.TempDir(), false, time.Second, &Logger{}),
		config: Config{DevToken: "secret"},
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/__nimbu/dev/templates/overlays", nil)
	req.Header.Set("X-Nimbu-Dev-Token", "secret")
	rec := httptest.NewRecorder()

	server.handleTemplateOverlays(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != "PUT, DELETE" {
		t.Fatalf("Allow = %q, want PUT, DELETE", got)
	}
}

type captureSimulatorClient struct {
	payload api.SimulatorPayload
}

func (c *captureSimulatorClient) SimulatorRender(_ context.Context, payload api.SimulatorPayload) (*api.SimulatorResponse, error) {
	c.payload = payload
	return &api.SimulatorResponse{
		Body:     base64.StdEncoding.EncodeToString([]byte("<html></html>")),
		Encoding: "base64",
		Headers:  map[string]string{"Content-Type": "text/html; charset=utf-8"},
		Status:   200,
	}, nil
}

func TestCatchAllUsesRegisteredTemplateOverlay(t *testing.T) {
	root := t.TempDir()
	layoutsDir := filepath.Join(root, "layouts")
	if err := os.MkdirAll(layoutsDir, 0o755); err != nil {
		t.Fatalf("mkdir layouts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(layoutsDir, "default.liquid"), []byte("layout"), 0o644); err != nil {
		t.Fatalf("write layout: %v", err)
	}

	client := &captureSimulatorClient{}
	server, err := New(Config{
		DevToken:     "secret",
		TemplateRoot: root,
		Watch:        false,
	}, client)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() { _ = server.Stop(context.Background()) }()

	overlayBody := `{"templates":[{"type":"snippets","path":"bundle_app.liquid","content":"virtual bundle"}]}`
	req, err := http.NewRequest(http.MethodPut, server.URL()+"/__nimbu/dev/templates/overlays", strings.NewReader(overlayBody))
	if err != nil {
		t.Fatalf("new overlay request: %v", err)
	}
	req.Header.Set("X-Nimbu-Dev-Token", "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put overlay: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overlay status = %d", resp.StatusCode)
	}

	resp, err = http.Get(server.URL() + "/")
	if err != nil {
		t.Fatalf("get page: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("page status = %d", resp.StatusCode)
	}

	templates := decodeCompressedTemplatesForTest(t, client.payload.Simulator.Code)
	if got := templates["snippets"]["bundle_app.liquid"]; got != "virtual bundle" {
		t.Fatalf("simulator payload overlay = %q", got)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}

	os.Stdout = w

	// Read in a goroutine to avoid deadlock on Windows where pipe buffers are small.
	done := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- data
	}()

	fn()

	_ = w.Close()
	os.Stdout = original

	data := <-done
	_ = r.Close()
	return string(data)
}
