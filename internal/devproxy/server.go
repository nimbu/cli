package devproxy

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nimbu/cli/internal/api"
)

// SimulatorClient is the API client dependency for simulator render calls.
type SimulatorClient interface {
	SimulatorRender(ctx context.Context, payload api.SimulatorPayload) (*api.SimulatorResponse, error)
}

// Config controls proxy server behavior.
type Config struct {
	APIURL            string
	Debug             bool
	DevToken          string
	EventsJSON        bool
	ExcludeRules      []string
	Host              string
	IncludeRules      []string
	MaxBodyBytes      int64
	Port              int
	QuietRequests     bool
	Site              string
	TemplateRoot      string
	UseColor          bool
	UserAgent         string
	Watch             bool
	WatchScanInterval time.Duration
}

// Server hosts the local simulator proxy.
type Server struct {
	cache   *TemplateCache
	client  SimulatorClient
	config  Config
	errCh   chan error
	logger  *Logger
	matcher Matcher

	httpServer *http.Server
	listener   net.Listener

	requestLogSpacer sync.Once
}

func New(config Config, client SimulatorClient) (*Server, error) {
	if client == nil {
		return nil, fmt.Errorf("simulator client is required")
	}
	if config.Host == "" {
		config.Host = "127.0.0.1"
	}
	if config.Port <= 0 {
		config.Port = 4568
	}
	if config.MaxBodyBytes <= 0 {
		config.MaxBodyBytes = 64 << 20
	}
	if config.TemplateRoot == "" {
		config.TemplateRoot = "."
	}
	if config.UserAgent == "" {
		config.UserAgent = "nimbu-go-cli"
	}

	logger := &Logger{DebugEnabled: config.Debug, EventsJSON: config.EventsJSON, UseColor: config.UseColor}
	cache := NewTemplateCache(config.TemplateRoot, config.Watch, config.WatchScanInterval, logger)
	matcher := NewMatcher(config.IncludeRules, config.ExcludeRules)

	return &Server{
		cache:   cache,
		client:  client,
		config:  config,
		errCh:   make(chan error, 1),
		logger:  logger,
		matcher: matcher,
	}, nil
}

func (s *Server) Start() error {
	if err := s.cache.Start(); err != nil {
		return fmt.Errorf("start template cache: %w", err)
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	handler := s.loggingMiddleware(mux)
	s.httpServer = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	listenAddr := net.JoinHostPort(s.config.Host, fmt.Sprintf("%d", s.config.Port))
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		_ = s.cache.Stop()
		return fmt.Errorf("listen on %s: %w", listenAddr, err)
	}
	s.listener = ln

	go func() {
		err := s.httpServer.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.errCh <- err
		}
		close(s.errCh)
	}()

	if s.config.EventsJSON {
		s.logger.Info("proxy server started", map[string]any{"url": s.URL()})
	}
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			_ = s.httpServer.Close()
		}
	}
	if err := s.cache.Stop(); err != nil {
		return err
	}
	return nil
}

func (s *Server) Errors() <-chan error {
	return s.errCh
}

func (s *Server) URL() string {
	if s.listener == nil {
		return ""
	}
	return "http://" + s.listener.Addr().String()
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/__nimbu/dev/templates/overlays", s.handleTemplateOverlays)

	for _, staticDir := range []string{"images", "fonts", "css", "stylesheets", "js", "javascripts"} {
		prefix := "/" + staticDir + "/"
		targetDir := filepath.Join(s.config.TemplateRoot, staticDir)
		fileServer := http.FileServer(http.Dir(targetDir))
		mux.Handle(prefix, http.StripPrefix(prefix, fileServer))
	}

	mux.HandleFunc("/", s.handleCatchAll)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	status := map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"nimbu": map[string]any{
			"api_url":       s.config.APIURL,
			"authenticated": true,
			"site":          s.config.Site,
		},
		"cache": s.cache.Status(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = jsonNewEncoder(w, status)
}

type templateOverlayRequest struct {
	Templates []TemplateOverlay `json:"templates"`
}

type templateOverlayResponse struct {
	OK    bool `json:"ok"`
	Count int  `json:"count"`
}

func (s *Server) handleTemplateOverlays(w http.ResponseWriter, req *http.Request) {
	if !validDevToken(s.config.DevToken, req.Header.Get("X-Nimbu-Dev-Token")) {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized", "invalid dev proxy token")
		return
	}

	switch req.Method {
	case http.MethodDelete:
		if err := s.cache.SetOverlays(nil); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Template Overlay Error", err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = jsonNewEncoder(w, templateOverlayResponse{OK: true, Count: 0})
	case http.MethodPut:
		var payload templateOverlayRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Bad Request", err.Error())
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			writeJSONError(w, http.StatusBadRequest, "Bad Request", "request body must contain a single JSON object")
			return
		}
		if err := s.cache.SetOverlays(payload.Templates); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Bad Request", err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = jsonNewEncoder(w, templateOverlayResponse{OK: true, Count: len(payload.Templates)})
	default:
		w.Header().Set("Allow", "PUT, DELETE")
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed", "overlay endpoint only accepts PUT or DELETE")
	}
}

func validDevToken(expected string, actual string) bool {
	if expected == "" || actual == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}

func (s *Server) handleCatchAll(w http.ResponseWriter, req *http.Request) {
	if !s.matcher.ShouldProxy(req.Method, req.URL.Path, isWebSocketUpgrade(req)) {
		writeJSONError(w, http.StatusNotFound, "Not Found", "Resource not handled by proxy server")
		return
	}

	rawBody, parsedBody, err := readBody(req, s.config.MaxBodyBytes)
	if err != nil {
		if errors.Is(err, errBodyTooLarge) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "Payload Too Large", "request body exceeds max_body_mb")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	templateCode, stale, err := s.cache.GetCompressed()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Template Error", err.Error())
		return
	}
	if stale {
		s.logger.Warn("using stale template cache after rebuild failure", nil)
	}

	payload, err := buildPayload(req, s.config.UserAgent, templateCode, rawBody, parsedBody)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Payload Error", err.Error())
		return
	}

	response, err := s.client.SimulatorRender(req.Context(), payload)
	if err != nil {
		s.handleSimulatorError(w, err)
		return
	}

	if err := writeSimulatorResponse(w, response); err != nil {
		writeJSONError(w, http.StatusBadGateway, "Proxy Error", err.Error())
		return
	}
}

func (s *Server) handleSimulatorError(w http.ResponseWriter, err error) {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		status := apiErr.StatusCode
		if status <= 0 {
			status = http.StatusBadGateway
		}
		writeJSONError(w, status, "Simulator API Error", apiErr.Message)
		return
	}

	message := err.Error()
	status := http.StatusBadGateway
	if strings.Contains(strings.ToLower(message), "refused") || strings.Contains(strings.ToLower(message), "timeout") {
		status = http.StatusServiceUnavailable
	}
	writeJSONError(w, status, "Proxy Error", message)
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, req)

		if s.config.QuietRequests {
			return
		}
		if req.URL.Path == "/health" {
			return
		}
		if !s.config.EventsJSON {
			s.requestLogSpacer.Do(func() {
				_, _ = fmt.Fprintln(os.Stdout)
			})
		}

		line := requestLogLine(strings.ToUpper(req.Method), requestPath(req), rec.status, s.config.UseColor && !s.config.EventsJSON)
		_, _ = fmt.Fprintln(os.Stdout, line)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func isWebSocketUpgrade(req *http.Request) bool {
	if req == nil {
		return false
	}
	if !strings.EqualFold(req.Method, http.MethodGet) {
		return false
	}
	if !strings.EqualFold(req.Header.Get("Upgrade"), "websocket") {
		return false
	}
	return headerHasToken(req.Header.Get("Connection"), "upgrade")
}

func headerHasToken(value string, token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	for _, part := range strings.Split(value, ",") {
		if strings.ToLower(strings.TrimSpace(part)) == token {
			return true
		}
	}
	return false
}

func jsonNewEncoder(w http.ResponseWriter, value any) error {
	return jsonEncoder(w).Encode(value)
}

// jsonEncoder split for testability.
var jsonEncoder = func(w http.ResponseWriter) interface{ Encode(v any) error } {
	return json.NewEncoder(w)
}
