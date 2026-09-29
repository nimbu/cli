package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// The tests in this file replay responses recorded from the Nimbu Rails
// OAuth endpoints (testdata/rails_contract.json) and check the requests the
// CLI sends against the wire contract those endpoints implement, so a drift
// on either side shows up here rather than in production.

type recordedResponse struct {
	Status      int             `json:"status"`
	ContentType string          `json:"content_type"`
	Body        json.RawMessage `json:"body"`
}

// railsServer serves the recorded responses. The API and admin hosts both
// map to the test server; answer picks the response for each request.
type railsServer struct {
	*httptest.Server
	t         *testing.T
	responses map[string]recordedResponse

	mu       sync.Mutex
	requests map[string][]url.Values
	answer   func(path string, form url.Values) string
}

func newRailsServer(t *testing.T, answer func(path string, form url.Values) string) *railsServer {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "rails_contract.json"))
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	rs := &railsServer{t: t, responses: map[string]recordedResponse{}, requests: map[string][]url.Values{}, answer: answer}
	for name, fixture := range fixtures {
		if strings.HasPrefix(name, "_") {
			continue
		}
		var resp recordedResponse
		if err := json.Unmarshal(fixture, &resp); err != nil {
			t.Fatalf("parse fixture %s: %v", name, err)
		}
		rs.responses[name] = resp
	}
	rs.Server = httptest.NewServer(http.HandlerFunc(rs.serve))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *railsServer) serve(w http.ResponseWriter, r *http.Request) {
	var name string
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/.well-known/oauth-authorization-server":
		name = "metadata"
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/oauth2/"):
		if r.Header.Get("Authorization") != "" {
			rs.t.Errorf("%s: public client sent an Authorization header", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			rs.t.Errorf("%s: Content-Type = %q, want form encoding", r.URL.Path, ct)
		}
		if err := r.ParseForm(); err != nil {
			rs.t.Errorf("%s: parse form: %v", r.URL.Path, err)
		}
		rs.mu.Lock()
		rs.requests[r.URL.Path] = append(rs.requests[r.URL.Path], r.PostForm)
		rs.mu.Unlock()
		name = rs.answer(r.URL.Path, r.PostForm)
	}
	resp, ok := rs.responses[name]
	if !ok {
		rs.t.Errorf("no recorded response for %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return
	}
	body := strings.NewReplacer("http://api.nimbu.test", rs.URL, "http://www.nimbu.test", rs.URL).Replace(string(resp.Body))
	w.Header().Set("Content-Type", resp.ContentType)
	w.WriteHeader(resp.Status)
	_, _ = w.Write([]byte(body))
}

func (rs *railsServer) forms(path string) []url.Values {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.requests[path]
}

func (rs *railsServer) client() *Client {
	return NewClient(rs.URL, rs.Client())
}

func formWithout(form url.Values, keys ...string) url.Values {
	out := url.Values{}
	for key, values := range form {
		out[key] = values
	}
	for _, key := range keys {
		out.Del(key)
	}
	return out
}

func TestRailsContractBrowserLogin(t *testing.T) {
	rs := newRailsServer(t, func(path string, _ url.Values) string {
		if path == "/oauth2/tokens" {
			return "token"
		}
		return ""
	})

	var authParams url.Values
	status := make(chan int, 1)
	tok, err := rs.client().LoginWithBrowser(context.Background(), BrowserLogin{
		Scopes:     []string{"read_channels", "write_themes"},
		DeviceName: "peters-mbp",
		Open: func(authURL string) error {
			parsed, err := url.Parse(authURL)
			if err != nil || parsed.Path != "/admin/oauth2/authorize" {
				t.Errorf("authorize url = %s", authURL)
			}
			authParams = parsed.Query()
			go func() {
				// Oauth2AuthorizeFlow#finalize_grant: code, state and the
				// REST issuer (RFC 9207) on the loopback redirect.
				status <- browserVisit(t, authURL, func(state string) url.Values {
					return url.Values{"code": {"auth-code"}, "state": {state}, "iss": {rs.URL}}
				})
			}()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("LoginWithBrowser: %v", err)
	}
	if got := <-status; got != http.StatusOK {
		t.Errorf("callback status = %d, want 200", got)
	}

	wantAuth := []string{"client_id", "code_challenge", "code_challenge_method", "device_name", "redirect_uri", "response_type", "scope", "state"}
	if gotAuth := slices.Sorted(maps.Keys(authParams)); !slices.Equal(gotAuth, wantAuth) {
		t.Errorf("authorize params = %v, want exactly %v", authParams, wantAuth)
	}
	for key, want := range map[string]string{
		"client_id":             "nimbu-cli",
		"response_type":         "code",
		"code_challenge_method": "S256",
		"device_name":           "peters-mbp",
		"scope":                 "read_channels write_themes",
	} {
		if got := authParams.Get(key); got != want {
			t.Errorf("authorize %s = %q, want %q", key, got, want)
		}
	}

	exchanges := rs.forms("/oauth2/tokens")
	if len(exchanges) != 1 {
		t.Fatalf("token requests = %v, want one code exchange", exchanges)
	}
	form := exchanges[0]
	want := url.Values{
		"grant_type":   {"authorization_code"},
		"client_id":    {"nimbu-cli"},
		"code":         {"auth-code"},
		"redirect_uri": {authParams.Get("redirect_uri")},
	}
	if got := formWithout(form, "code_verifier"); !reflect.DeepEqual(got, want) {
		t.Errorf("code exchange form = %v, want %v + code_verifier", form, want)
	}
	if oauth2.S256ChallengeFromVerifier(form.Get("code_verifier")) != authParams.Get("code_challenge") {
		t.Error("code_verifier does not match the code_challenge")
	}

	// rack-oauth2 answers token_type "bearer" in lower case.
	if tok.Type() != "Bearer" {
		t.Errorf("token type = %q, want Bearer", tok.Type())
	}
	cred := Credential(tok, "nimbu-cli")
	if cred.RefreshToken == "" || !reflect.DeepEqual(cred.Scopes, []string{"write_channels", "write_content", "write_themes"}) {
		t.Errorf("credential = %+v", cred)
	}
	if left := time.Until(cred.ExpiresAt); left < 29*time.Minute || left > 30*time.Minute {
		t.Errorf("expires_at in %v, want ~30 minutes", left)
	}
}

func TestRailsContractBrowserLoginRejectsAnotherIssuer(t *testing.T) {
	rs := newRailsServer(t, func(string, url.Values) string { return "token" })

	status := make(chan int, 1)
	_, err := rs.client().LoginWithBrowser(context.Background(), BrowserLogin{
		Timeout: 5 * time.Second,
		Open: func(authURL string) error {
			go func() {
				status <- browserVisit(t, authURL, func(state string) url.Values {
					// The MCP authorization server's issuer, not the REST one.
					return url.Values{"code": {"auth-code"}, "state": {state}, "iss": {rs.URL + "/mcp"}}
				})
			}()
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected issuer") {
		t.Fatalf("error = %v, want an issuer mismatch", err)
	}
	if got := <-status; got != http.StatusBadRequest {
		t.Errorf("callback status = %d, want 400", got)
	}
	if exchanges := rs.forms("/oauth2/tokens"); len(exchanges) != 0 {
		t.Fatalf("code exchanged despite the issuer mismatch: %v", exchanges)
	}
}

// deviceContractServer rewrites the recorded 5-second interval to 1 second
// so the test does not wait on the real cadence.
func deviceContractServer(t *testing.T, polls ...string) *railsServer {
	t.Helper()
	var mu sync.Mutex
	n := 0
	rs := newRailsServer(t, func(path string, _ url.Values) string {
		switch path {
		case "/oauth2/device_authorization":
			return "device_authorization"
		case "/oauth2/tokens":
			mu.Lock()
			defer mu.Unlock()
			n++
			return polls[min(n, len(polls))-1]
		}
		return ""
	})
	resp := rs.responses["device_authorization"]
	resp.Body = json.RawMessage(strings.Replace(string(resp.Body), `"interval":5`, `"interval":1`, 1))
	rs.responses["device_authorization"] = resp
	return rs
}

func TestRailsContractDeviceLogin(t *testing.T) {
	t.Parallel()
	rs := deviceContractServer(t, "authorization_pending", "token")

	var shownURI, shownCode string
	tok, err := rs.client().LoginWithDevice(context.Background(), DeviceLogin{
		Scopes:     []string{"read_channels"},
		DeviceName: "build-box",
		Prompt: func(uri, code string, _ time.Time) {
			shownURI, shownCode = uri, code
		},
	})
	if err != nil {
		t.Fatalf("LoginWithDevice: %v", err)
	}
	if tok.AccessToken == "" || tok.RefreshToken == "" {
		t.Fatalf("token = %+v", tok)
	}
	if shownURI != rs.URL+"/admin/oauth2/device" || shownCode != "RDQR-RDRS" {
		t.Errorf("prompt = %q %q", shownURI, shownCode)
	}

	starts := rs.forms("/oauth2/device_authorization")
	wantStart := url.Values{"client_id": {"nimbu-cli"}, "scope": {"read_channels"}, "device_name": {"build-box"}}
	if len(starts) != 1 || !reflect.DeepEqual(starts[0], wantStart) {
		t.Errorf("device authorization forms = %v, want [%v]", starts, wantStart)
	}
	wantPoll := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":   {"nimbu-cli"},
		"device_code": {"k3yJ5v0o4dXn9Qe2mWlZ8rTgYhBuC1aFsE6iPx7NqL0"},
	}
	polls := rs.forms("/oauth2/tokens")
	if len(polls) != 2 {
		t.Fatalf("polls = %d, want 2", len(polls))
	}
	for _, form := range polls {
		if !reflect.DeepEqual(form, wantPoll) {
			t.Errorf("poll form = %v, want %v", form, wantPoll)
		}
	}
}

func TestRailsContractDeviceLoginErrors(t *testing.T) {
	t.Parallel()
	cases := map[string]func(error) bool{
		"access_denied":        func(err error) bool { return errors.Is(err, ErrDeviceDenied) },
		"expired_token":        func(err error) bool { return errors.Is(err, ErrDeviceExpired) },
		"device_invalid_grant": IsInvalidGrant,
	}
	for fixture, match := range cases {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			rs := deviceContractServer(t, fixture)
			_, err := rs.client().LoginWithDevice(context.Background(), DeviceLogin{})
			if err == nil || !match(err) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestRailsContractRefresh(t *testing.T) {
	rs := newRailsServer(t, func(_ string, form url.Values) string {
		if form.Get("refresh_token") == "dead" {
			return "invalid_grant"
		}
		return "token"
	})

	tok, err := rs.client().Refresh(context.Background(), "live")
	if err != nil || tok.RefreshToken == "live" {
		t.Fatalf("Refresh = %+v, %v; want a rotated refresh token", tok, err)
	}
	if _, err := rs.client().Refresh(context.Background(), "dead"); !IsInvalidGrant(err) {
		t.Fatalf("Refresh(dead) error = %v, want invalid_grant", err)
	}

	want := url.Values{"grant_type": {"refresh_token"}, "client_id": {"nimbu-cli"}, "refresh_token": {"live"}}
	if forms := rs.forms("/oauth2/tokens"); len(forms) != 2 || !reflect.DeepEqual(forms[0], want) {
		t.Fatalf("refresh forms = %v, want %v first", forms, want)
	}
}

func TestRailsContractRevoke(t *testing.T) {
	rs := newRailsServer(t, func(path string, _ url.Values) string {
		if path == "/oauth2/revoke" {
			return "revoke"
		}
		return ""
	})

	if err := rs.client().Revoke(context.Background(), "refresh-1", "refresh_token"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	want := url.Values{"token": {"refresh-1"}, "token_type_hint": {"refresh_token"}, "client_id": {"nimbu-cli"}}
	if forms := rs.forms("/oauth2/revoke"); len(forms) != 1 || !reflect.DeepEqual(forms[0], want) {
		t.Fatalf("revoke forms = %v, want [%v]", forms, want)
	}
}

func TestRailsContractDiscovery(t *testing.T) {
	rs := newRailsServer(t, func(string, url.Values) string { return "" })

	got := rs.client().Endpoints(context.Background())
	want := Endpoints{
		Issuer:                      rs.URL,
		AuthorizationEndpoint:       rs.URL + "/admin/oauth2/authorize",
		TokenEndpoint:               rs.URL + "/oauth2/tokens",
		DeviceAuthorizationEndpoint: rs.URL + "/oauth2/device_authorization",
		RevocationEndpoint:          rs.URL + "/oauth2/revoke",
	}
	if got != want {
		t.Fatalf("Endpoints = %+v, want %+v", got, want)
	}
}
