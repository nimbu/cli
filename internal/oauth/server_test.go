package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// fakeServer is a minimal authorization server speaking the Nimbu wire
// contract. Handlers for the token and device endpoints are set per test.
type fakeServer struct {
	*httptest.Server
	t *testing.T

	mu       sync.Mutex
	token    func(form url.Values) (int, map[string]any)
	device   func(form url.Values) (int, map[string]any)
	revoked  []url.Values
	requests []url.Values
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	fs := &fakeServer{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                        fs.URL,
			"authorization_endpoint":        fs.URL + "/admin/oauth2/authorize",
			"token_endpoint":                fs.URL + "/oauth2/tokens",
			"device_authorization_endpoint": fs.URL + "/oauth2/device_authorization",
			"revocation_endpoint":           fs.URL + "/oauth2/revoke",
		})
	})
	mux.HandleFunc("POST /oauth2/tokens", func(w http.ResponseWriter, r *http.Request) {
		form := fs.form(r)
		fs.mu.Lock()
		handler := fs.token
		fs.mu.Unlock()
		if handler == nil {
			t.Errorf("unexpected token request: %v", form)
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
			return
		}
		status, body := handler(form)
		writeJSON(w, status, body)
	})
	mux.HandleFunc("POST /oauth2/device_authorization", func(w http.ResponseWriter, r *http.Request) {
		form := fs.form(r)
		fs.mu.Lock()
		handler := fs.device
		fs.mu.Unlock()
		status, body := handler(form)
		writeJSON(w, status, body)
	})
	mux.HandleFunc("POST /oauth2/revoke", func(w http.ResponseWriter, r *http.Request) {
		form := fs.form(r)
		fs.mu.Lock()
		fs.revoked = append(fs.revoked, form)
		fs.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{})
	})
	fs.Server = httptest.NewServer(mux)
	t.Cleanup(fs.Close)
	return fs
}

func (fs *fakeServer) form(r *http.Request) url.Values {
	fs.t.Helper()
	if got := r.Header.Get("Authorization"); got != "" {
		fs.t.Errorf("public client must not send Authorization, got %q", got)
	}
	if err := r.ParseForm(); err != nil {
		fs.t.Errorf("parse form: %v", err)
	}
	fs.mu.Lock()
	fs.requests = append(fs.requests, r.PostForm)
	fs.mu.Unlock()
	return r.PostForm
}

func (fs *fakeServer) setToken(h func(form url.Values) (int, map[string]any)) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.token = h
}

func (fs *fakeServer) setDevice(h func(form url.Values) (int, map[string]any)) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.device = h
}

func (fs *fakeServer) client() *Client {
	return NewClient(fs.URL, fs.Client())
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func tokenResponse(access, refresh string) map[string]any {
	return map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    1800,
		"refresh_token": refresh,
		"scope":         "read_channels write_channels",
	}
}
