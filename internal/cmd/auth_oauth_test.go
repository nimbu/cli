package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/oauth"
	"github.com/nimbu/cli/internal/output"
)

// oauthAPI is a fake Nimbu API plus authorization server. It serves no
// discovery document, so the CLI falls back to the conventional endpoints.
type oauthAPI struct {
	*httptest.Server
	refreshes atomic.Int32
	revoked   atomic.Value // url.Values
	polls     atomic.Int32
	logouts   atomic.Int32 // legacy POST /auth/logout
}

func newOAuthAPI(t *testing.T) *oauthAPI {
	t.Helper()
	fake := &oauthAPI{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth2/tokens", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			writeTestJSON(w, http.StatusOK, testTokenResponse("access-1", "refresh-1"))
		case "urn:ietf:params:oauth:grant-type:device_code":
			fake.polls.Add(1)
			writeTestJSON(w, http.StatusOK, testTokenResponse("access-1", "refresh-1"))
		case "refresh_token":
			if r.PostForm.Get("refresh_token") != "refresh-1" {
				writeTestJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
				return
			}
			fake.refreshes.Add(1)
			writeTestJSON(w, http.StatusOK, testTokenResponse("access-2", "refresh-2"))
		default:
			t.Errorf("unexpected grant: %v", r.PostForm)
		}
	})
	mux.HandleFunc("POST /oauth2/device_authorization", func(w http.ResponseWriter, r *http.Request) {
		writeTestJSON(w, http.StatusOK, map[string]any{
			"device_code":      "device-secret",
			"user_code":        "BCDF-GHJK",
			"verification_uri": "https://www.example.test/admin/oauth2/device",
			"expires_in":       600,
			"interval":         1,
		})
	})
	mux.HandleFunc("POST /oauth2/revoke", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.revoked.Store(r.PostForm)
		writeTestJSON(w, http.StatusOK, map[string]any{})
	})
	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		fake.logouts.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer access-1", "Bearer access-2":
			writeTestJSON(w, http.StatusOK, map[string]any{"id": "u1", "email": "me@example.com", "name": "Me"})
		default:
			writeTestJSON(w, http.StatusUnauthorized, map[string]any{"message": "unauthorized"})
		}
	})
	fake.Server = httptest.NewServer(mux)
	t.Cleanup(fake.Close)
	return fake
}

func writeTestJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func testTokenResponse(access, refresh string) map[string]any {
	return map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": 1800, "refresh_token": refresh, "scope": "read_channels"}
}

// oauthTestContext builds a command context against apiURL with buffered
// output and an isolated refresh lock directory.
func oauthTestContext(t *testing.T, apiURL string, mode output.Mode) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	lockDir := t.TempDir()
	oldLockDir := refreshLockDir
	refreshLockDir = func() (string, error) { return lockDir, nil }
	t.Cleanup(func() { refreshLockDir = oldLockDir })

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, &RootFlags{APIURL: apiURL, Timeout: 10 * time.Second})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})
	ctx = context.WithValue(ctx, authResolverKey{}, newAuthCredentialResolver(hostOf(apiURL)))
	ctx = output.WithMode(ctx, mode)
	ctx = output.WithWriter(ctx, &output.Writer{Out: &stdout, Err: &stderr, Mode: mode})
	return ctx, &stdout, &stderr
}

func hostOf(rawURL string) string {
	parsed, _ := url.Parse(rawURL)
	return parsed.Host
}

// withBrowserEnv makes headless detection pick the browser flow everywhere.
func withBrowserEnv(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	t.Setenv("DISPLAY", ":0")
}

func TestAuthLoginBrowserStoresOAuthSessionAndRevokesPrevious(t *testing.T) {
	withBrowserEnv(t)
	fake := newOAuthAPI(t)
	store := &fakeAuthStore{credential: auth.Credential{
		Token: "old-access", AuthMethod: auth.AuthMethodOAuth, RefreshToken: "old-refresh",
	}}
	withFakeAuthStore(t, store)

	oldOpen := openBrowser
	openBrowser = func(authURL string) error {
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		query := parsed.Query()
		go func() {
			callback := query.Get("redirect_uri") + "?" + url.Values{"code": {"code-1"}, "state": {query.Get("state")}}.Encode()
			if resp, err := http.Get(callback); err == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}
	t.Cleanup(func() { openBrowser = oldOpen })

	ctx, stdout, stderr := oauthTestContext(t, fake.URL, output.Mode{JSON: true})
	if err := (&AuthLoginCmd{}).Run(ctx, ctx.Value(rootFlagsKey{}).(*RootFlags)); err != nil {
		t.Fatalf("auth login: %v", err)
	}

	cred := store.lastCredential
	if !cred.IsOAuth() || cred.Token != "access-1" || cred.RefreshToken != "refresh-1" || cred.Email != "me@example.com" || cred.ClientID != oauth.DefaultClientID {
		t.Fatalf("stored credential = %+v", cred)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json output %q: %v", stdout.String(), err)
	}
	if payload["auth_method"] != "oauth" || payload["email"] != "me@example.com" || payload["host"] != hostOf(fake.URL) || payload["expires_at"] == nil {
		t.Fatalf("json payload = %v", payload)
	}
	if revoked, _ := fake.revoked.Load().(url.Values); revoked.Get("token") != "old-refresh" {
		t.Fatalf("previous session not revoked: %v", revoked)
	}
	if !strings.Contains(stderr.String(), "/admin/oauth2/authorize?") {
		t.Fatalf("stderr does not show the authorization URL: %q", stderr.String())
	}
}

func TestAuthLoginDeviceShowsCodeAndStoresSession(t *testing.T) {
	withBrowserEnv(t)
	fake := newOAuthAPI(t)
	store := &fakeAuthStore{credentialErr: auth.ErrNoToken}
	withFakeAuthStore(t, store)
	oldOpen := openBrowser
	openBrowser = func(string) error {
		t.Error("device login must not open a browser")
		return nil
	}
	t.Cleanup(func() { openBrowser = oldOpen })

	ctx, stdout, stderr := oauthTestContext(t, fake.URL, output.Mode{})
	if err := (&AuthLoginCmd{Device: true}).Run(ctx, ctx.Value(rootFlagsKey{}).(*RootFlags)); err != nil {
		t.Fatalf("auth login --device: %v", err)
	}
	if !strings.Contains(stderr.String(), "BCDF-GHJK") || !strings.Contains(stderr.String(), "https://www.example.test/admin/oauth2/device") {
		t.Fatalf("stderr = %q, want the code and verification URI", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Logged in to "+hostOf(fake.URL)+" as Me (me@example.com)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !store.lastCredential.IsOAuth() || fake.polls.Load() != 1 {
		t.Fatalf("stored = %+v after %d polls", store.lastCredential, fake.polls.Load())
	}
}

func TestAPIClientRefreshesOAuthSessionOn401(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "")
	fake := newOAuthAPI(t)
	store := &fakeAuthStore{credential: auth.Credential{
		Token:        "revoked-access",
		AuthMethod:   auth.AuthMethodOAuth,
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(20 * time.Minute),
		Email:        "me@example.com",
	}}
	withFakeAuthStore(t, store)
	ctx, _, _ := oauthTestContext(t, fake.URL, output.Mode{})

	client, err := GetAPIClient(ctx)
	if err != nil {
		t.Fatalf("GetAPIClient: %v", err)
	}
	var user map[string]any
	if err := client.WithSite("demo").Get(ctx, "/user", &user); err != nil {
		t.Fatalf("GET /user: %v", err)
	}
	if fake.refreshes.Load() != 1 || store.lastCredential.Token != "access-2" || store.lastCredential.RefreshToken != "refresh-2" {
		t.Fatalf("refreshes = %d, stored = %+v", fake.refreshes.Load(), store.lastCredential)
	}
	if store.lastCredential.Email != "me@example.com" {
		t.Fatalf("refresh dropped the email: %+v", store.lastCredential)
	}
	if token, err := ResolveAuthToken(ctx); err != nil || token != "access-2" {
		t.Fatalf("ResolveAuthToken after refresh = %q, %v", token, err)
	}
}

func TestAPIClientInvalidGrantMeansLoggedOut(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "")
	fake := newOAuthAPI(t)
	store := &fakeAuthStore{credential: auth.Credential{
		Token:        "revoked-access",
		AuthMethod:   auth.AuthMethodOAuth,
		RefreshToken: "revoked-refresh",
		ExpiresAt:    time.Now().Add(20 * time.Minute),
	}}
	withFakeAuthStore(t, store)
	ctx, _, _ := oauthTestContext(t, fake.URL, output.Mode{})

	client, err := GetAPIClient(ctx)
	if err != nil {
		t.Fatalf("GetAPIClient: %v", err)
	}
	err = client.Get(ctx, "/user", nil)
	if !errors.Is(err, oauth.ErrSessionExpired) {
		t.Fatalf("error = %v, want ErrSessionExpired", err)
	}
	desc := classifyError(err)
	if desc.Code != errorAuthNotLoggedIn || desc.ExitCode != ExitAuth || desc.Hint != "run `nimbu auth login`" {
		t.Fatalf("error contract = %+v", desc)
	}
	if store.deleteCredentialCalls != 1 {
		t.Fatalf("delete credential calls = %d, want 1", store.deleteCredentialCalls)
	}
}

func TestAuthTokenRefreshesExpiredSession(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "")
	fake := newOAuthAPI(t)
	store := &fakeAuthStore{credential: auth.Credential{
		Token:        "expired-access",
		AuthMethod:   auth.AuthMethodOAuth,
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}}
	withFakeAuthStore(t, store)
	ctx, stdout, _ := oauthTestContext(t, fake.URL, output.Mode{})

	if err := (&AuthTokenCmd{}).Run(ctx); err != nil {
		t.Fatalf("auth token: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "access-2" {
		t.Fatalf("auth token printed %q, want the refreshed token", got)
	}
}

func TestAuthLogoutRevokesOAuthSessionBestEffort(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "")
	cases := map[string]bool{"revoke succeeds": true, "server unreachable": false}
	for name, reachable := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newOAuthAPI(t)
			apiURL := fake.URL
			if !reachable {
				fake.Close()
			}
			store := &fakeAuthStore{credential: auth.Credential{
				Token: "access-1", AuthMethod: auth.AuthMethodOAuth, RefreshToken: "refresh-1", ClientID: "nimbu-cli",
			}}
			withFakeAuthStore(t, store)
			ctx, stdout, stderr := oauthTestContext(t, apiURL, output.Mode{})
			ctx.Value(rootFlagsKey{}).(*RootFlags).Readonly = true

			if err := (&AuthLogoutCmd{}).Run(ctx); err != nil {
				t.Fatalf("auth logout: %v", err)
			}
			if store.deleteCredentialCalls != 1 || !strings.Contains(stdout.String(), "Logged out") {
				t.Fatalf("deletes = %d, stdout = %q", store.deleteCredentialCalls, stdout.String())
			}
			if reachable {
				want := url.Values{"token": {"refresh-1"}, "token_type_hint": {"refresh_token"}, "client_id": {"nimbu-cli"}}
				if got, _ := fake.revoked.Load().(url.Values); !reflect.DeepEqual(got, want) {
					t.Fatalf("revoke form = %v, want %v", got, want)
				}
			} else if !strings.Contains(stderr.String(), "could not revoke") {
				t.Fatalf("stderr = %q, want a revoke warning", stderr.String())
			}
		})
	}
}

func TestIsHeadless(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		goos string
		want bool
	}{
		{"macOS desktop", nil, "darwin", false},
		{"windows desktop", nil, "windows", false},
		{"ssh session", map[string]string{"SSH_CONNECTION": "1.2.3.4 5 6.7.8.9 22"}, "darwin", true},
		{"ssh tty", map[string]string{"SSH_TTY": "/dev/pts/0"}, "windows", true},
		{"linux without display", nil, "linux", true},
		{"linux x11", map[string]string{"DISPLAY": ":0"}, "linux", false},
		{"linux wayland", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, "linux", false},
	}
	for _, tc := range cases {
		getenv := func(key string) string { return tc.env[key] }
		if got := isHeadless(getenv, tc.goos); got != tc.want {
			t.Errorf("%s: isHeadless = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSplitScopes(t *testing.T) {
	got := splitScopes("read_channels, write_channels read_channels,,")
	want := []string{"read_channels", "write_channels"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitScopes = %q, want %q", got, want)
	}
	if splitScopes("") != nil {
		t.Fatal("empty scopes must not request any scope")
	}
}

func TestAuthLoginPasswordFlagsUseDeprecatedLegacyFlow(t *testing.T) {
	withBrowserEnv(t)
	var legacyLogins atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/login" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			return
		}
		legacyLogins.Add(1)
		writeTestJSON(w, http.StatusOK, map[string]any{"token": "legacy-token"})
	}))
	defer srv.Close()
	store := &fakeAuthStore{credentialErr: auth.ErrNoToken}
	withFakeAuthStore(t, store)

	ctx, _, stderr := oauthTestContext(t, srv.URL, output.Mode{})
	cmd := &AuthLoginCmd{Email: "me@example.com", Password: "sekret", ExpiresIn: 60}
	if err := cmd.Run(ctx, ctx.Value(rootFlagsKey{}).(*RootFlags)); err != nil {
		t.Fatalf("auth login --email: %v", err)
	}
	if legacyLogins.Load() != 1 || store.lastCredential.Token != "legacy-token" || store.lastCredential.IsOAuth() {
		t.Fatalf("logins = %d, stored = %+v", legacyLogins.Load(), store.lastCredential)
	}
	if !strings.Contains(stderr.String(), "deprecated") {
		t.Fatalf("stderr = %q, want a deprecation warning", stderr.String())
	}
}
