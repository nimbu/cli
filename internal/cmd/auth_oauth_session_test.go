package cmd

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"

	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/oauth"
	"github.com/nimbu/cli/internal/output"
)

func oauthSession(access, refresh string) auth.Credential {
	return auth.Credential{
		Token: access, AuthMethod: auth.AuthMethodOAuth, RefreshToken: refresh,
		ExpiresAt: time.Now().Add(20 * time.Minute), Email: "me@example.com", ClientID: "nimbu-cli",
		Scopes: []string{"read_channels"},
	}
}

func decodeJSONOutput(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json output %q: %v", raw, err)
	}
	return payload
}

func TestAuthStatusShowsOAuthSession(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "")
	fake := newOAuthAPI(t)
	withFakeAuthStore(t, &fakeAuthStore{credential: oauthSession("access-1", "refresh-1")})
	ctx, stdout, _ := oauthTestContext(t, fake.URL, output.Mode{JSON: true})

	if err := (&AuthStatusCmd{}).Run(ctx, ctx.Value(rootFlagsKey{}).(*RootFlags)); err != nil {
		t.Fatalf("auth status: %v", err)
	}
	payload := decodeJSONOutput(t, stdout.Bytes())
	if payload["logged_in"] != true || payload["verified"] != true || payload["auth_method"] != "oauth" || payload["expires_at"] == nil {
		t.Fatalf("status = %v", payload)
	}
}

func TestAuthStatusReportsRevokedSessionAsLoggedOut(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "")
	fake := newOAuthAPI(t)
	store := &fakeAuthStore{credential: oauthSession("revoked-access", "revoked-refresh")}
	withFakeAuthStore(t, store)

	for _, mode := range []output.Mode{{JSON: true}, {}} {
		store.credential, store.credentialErr = oauthSession("revoked-access", "revoked-refresh"), nil
		ctx, stdout, _ := oauthTestContext(t, fake.URL, mode)
		if err := (&AuthStatusCmd{}).Run(ctx, ctx.Value(rootFlagsKey{}).(*RootFlags)); err != nil {
			t.Fatalf("auth status: %v", err)
		}
		if mode.JSON {
			payload := decodeJSONOutput(t, stdout.Bytes())
			if payload["logged_in"] != false || payload["reason"] != "session_expired" {
				t.Fatalf("status = %v, want logged out with an expired session", payload)
			}
		} else if !strings.Contains(stdout.String(), "Not logged in") || !strings.Contains(stdout.String(), "expired or was revoked") {
			t.Fatalf("status = %q", stdout.String())
		}
	}
	if store.deleteCredentialCalls != 2 {
		t.Fatalf("delete credential calls = %d, want 2", store.deleteCredentialCalls)
	}
}

func TestAuthLogoutWithEnvTokenRevokesStoredOAuthSession(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "ci-token")
	fake := newOAuthAPI(t)
	store := &fakeAuthStore{credential: oauthSession("access-1", "refresh-1")}
	withFakeAuthStore(t, store)
	ctx, _, stderr := oauthTestContext(t, fake.URL, output.Mode{})

	if err := (&AuthLogoutCmd{}).Run(ctx); err != nil {
		t.Fatalf("auth logout: %v", err)
	}
	if revoked, _ := fake.revoked.Load().(url.Values); revoked.Get("token") != "refresh-1" {
		t.Fatalf("stored session not revoked: %v", revoked)
	}
	if fake.logouts.Load() != 0 {
		t.Fatal("logout must not revoke NIMBU_TOKEN through /auth/logout")
	}
	if store.deleteCredentialCalls != 1 || !strings.Contains(stderr.String(), "NIMBU_TOKEN is still set") {
		t.Fatalf("deletes = %d, stderr = %q", store.deleteCredentialCalls, stderr.String())
	}
}

func TestAuthLogoutWithOnlyEnvTokenLeavesItAlone(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "ci-token")
	fake := newOAuthAPI(t)
	withFakeAuthStore(t, &fakeAuthStore{credentialErr: auth.ErrNoToken, tokenErr: auth.ErrNoToken})
	ctx, _, _ := oauthTestContext(t, fake.URL, output.Mode{})

	err := (&AuthLogoutCmd{}).Run(ctx)
	if !errors.Is(err, auth.ErrNoToken) || !strings.Contains(err.Error(), "NIMBU_TOKEN") {
		t.Fatalf("auth logout error = %v, want a not-logged-in error naming NIMBU_TOKEN", err)
	}
	if fake.logouts.Load() != 0 || fake.revoked.Load() != nil {
		t.Fatal("logout must not revoke NIMBU_TOKEN")
	}
}

// TestAuthLoginWaitsForARefreshInAnotherProcess holds the session lock like a
// refreshing process would: the login must not replace the session (nor read
// the one to revoke) until that refresh has saved its rotation.
func TestAuthLoginWaitsForARefreshInAnotherProcess(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "")
	store := &fakeAuthStore{credential: oauthSession("access-0", "refresh-0")}
	withFakeAuthStore(t, store)
	ctx, _, _ := oauthTestContext(t, "https://api.example.test", output.Mode{})
	host := "api.example.test"

	unlock, err := oauth.LockSession(ctx, refreshLockPath(host))
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	type result struct {
		previous auth.Credential
		err      error
	}
	done := make(chan result, 1)
	go func() {
		previous, err := replaceStoredCredential(ctx, host, oauthSession("access-new", "refresh-new"))
		done <- result{previous, err}
	}()

	select {
	case <-done:
		t.Fatal("login replaced the session while another process held the lock")
	case <-time.After(200 * time.Millisecond):
	}
	// The other process finishes its refresh and releases the lock.
	_ = store.SetCredential(oauthSession("access-1", "refresh-1"))
	unlock()

	got := <-done
	if got.err != nil || got.previous.RefreshToken != "refresh-1" {
		t.Fatalf("previous = %+v, %v; want the rotated session to revoke", got.previous, got.err)
	}
	if store.lastCredential.RefreshToken != "refresh-new" {
		t.Fatalf("stored = %+v", store.lastCredential)
	}
}

// TestAuthLoginIgnoresNIMBUTokenEnv guards the flag binding: an exported
// NIMBU_TOKEN must not turn `nimbu auth login` into storing that token.
func TestAuthLoginIgnoresNIMBUTokenEnv(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "ci-token")
	var cli struct {
		Login AuthLoginCmd `cmd:""`
	}
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	if _, err := parser.Parse([]string{"login", "--device"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cli.Login.Token != "" || !cli.Login.Device {
		t.Fatalf("parsed login = %+v, want the device flow without a token", cli.Login)
	}
}

func TestAuthLoginWithNIMBUTokenSetLogsInAndWarns(t *testing.T) {
	withBrowserEnv(t)
	t.Setenv("NIMBU_TOKEN", "ci-token")
	fake := newOAuthAPI(t)
	store := &fakeAuthStore{credentialErr: auth.ErrNoToken}
	withFakeAuthStore(t, store)
	ctx, _, stderr := oauthTestContext(t, fake.URL, output.Mode{})

	if err := (&AuthLoginCmd{Device: true}).Run(ctx, ctx.Value(rootFlagsKey{}).(*RootFlags)); err != nil {
		t.Fatalf("auth login --device: %v", err)
	}
	if !store.lastCredential.IsOAuth() || store.lastCredential.Token == "ci-token" {
		t.Fatalf("stored = %+v, want the new OAuth session", store.lastCredential)
	}
	if !strings.Contains(stderr.String(), "NIMBU_TOKEN is set and overrides this login") {
		t.Fatalf("stderr = %q, want a NIMBU_TOKEN warning", stderr.String())
	}
}

// TestAuthLoginWithTokenRevokesReplacedOAuthSession covers every non-OAuth
// login path: replacing a stored OAuth session must end it on the server.
func TestAuthLoginWithTokenRevokesReplacedOAuthSession(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "")
	fake := newOAuthAPI(t)
	store := &fakeAuthStore{credential: oauthSession("access-1", "refresh-1")}
	withFakeAuthStore(t, store)
	ctx, _, _ := oauthTestContext(t, fake.URL, output.Mode{})

	if err := (&AuthLoginCmd{Token: "api-token"}).Run(ctx, ctx.Value(rootFlagsKey{}).(*RootFlags)); err != nil {
		t.Fatalf("auth login --token: %v", err)
	}
	if store.lastCredential.Token != "api-token" || store.lastCredential.IsOAuth() {
		t.Fatalf("stored = %+v", store.lastCredential)
	}
	if revoked, _ := fake.revoked.Load().(url.Values); revoked.Get("token") != "refresh-1" || revoked.Get("client_id") != "nimbu-cli" {
		t.Fatalf("replaced OAuth session not revoked: %v", revoked)
	}
}

func TestClassifyRefreshFailures(t *testing.T) {
	cases := []struct {
		err       error
		code      canonicalErrorCode
		exit      int
		retryable bool
	}{
		{&oauth.RefreshError{Status: 401, Code: "invalid_client"}, errorAuthUnauthorized, ExitAuth, false},
		{&oauth.RefreshError{Status: 502}, errorServerError, ExitGeneral, true},
		{&oauth.RefreshError{Status: 429}, errorRateLimited, ExitRateLimit, true},
	}
	for _, tc := range cases {
		desc := classifyError(tc.err)
		if desc.Code != tc.code || desc.ExitCode != tc.exit || desc.Retryable != tc.retryable {
			t.Errorf("classifyError(%v) = %+v", tc.err, desc)
		}
	}
	if desc := classifyError(&oauth.RefreshError{Status: 400, Code: "invalid_client"}); desc.Hint != "run `nimbu auth login`" {
		t.Errorf("rejected refresh hint = %q", desc.Hint)
	}
}
