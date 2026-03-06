package cmd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

type fakeAuthStore struct {
	mu sync.Mutex

	credentialDelay time.Duration

	credential    auth.Credential
	credentialErr error
	token         string
	tokenErr      error
	email         string
	emailErr      error

	deleteCredentialErr error
	setCredentialErr    error

	getCredentialCalls    int
	getTokenCalls         int
	getEmailCalls         int
	setCredentialCalls    int
	setTokenCalls         int
	setEmailCalls         int
	deleteCredentialCalls int

	lastCredential auth.Credential
}

func (s *fakeAuthStore) SetCredential(cred auth.Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCredentialCalls++
	s.lastCredential = cred
	if s.setCredentialErr != nil {
		return s.setCredentialErr
	}
	s.credential = cred
	s.credentialErr = nil
	return nil
}

func (s *fakeAuthStore) GetCredential() (auth.Credential, error) {
	s.mu.Lock()
	s.getCredentialCalls++
	cred := s.credential
	err := s.credentialErr
	delay := s.credentialDelay
	token := strings.TrimSpace(s.token)
	email := strings.ToLower(strings.TrimSpace(s.email))
	emailErr := s.emailErr
	s.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}
	if err != nil {
		if errors.Is(err, auth.ErrNoToken) && token != "" && (emailErr == nil || errors.Is(emailErr, auth.ErrNoEmail)) {
			cred = auth.Credential{Token: token, Email: email}
			_ = s.SetCredential(cred)
			return cred, nil
		}
		return auth.Credential{}, err
	}
	return cred, nil
}

func (s *fakeAuthStore) DeleteCredential() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteCredentialCalls++
	if s.deleteCredentialErr != nil {
		return s.deleteCredentialErr
	}
	s.credential = auth.Credential{}
	s.credentialErr = auth.ErrNoToken
	s.token = ""
	s.tokenErr = auth.ErrNoToken
	s.email = ""
	s.emailErr = auth.ErrNoEmail
	return nil
}

func (s *fakeAuthStore) SetToken(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setTokenCalls++
	s.token = token
	s.tokenErr = nil
	return nil
}

func (s *fakeAuthStore) GetToken() (string, error) {
	s.mu.Lock()
	s.getTokenCalls++
	token := s.token
	err := s.tokenErr
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *fakeAuthStore) DeleteToken() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = ""
	s.tokenErr = auth.ErrNoToken
	return nil
}

func (s *fakeAuthStore) SetEmail(email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setEmailCalls++
	s.email = email
	s.emailErr = nil
	return nil
}

func (s *fakeAuthStore) GetEmail() (string, error) {
	s.mu.Lock()
	s.getEmailCalls++
	email := s.email
	err := s.emailErr
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	return email, nil
}

func (s *fakeAuthStore) DeleteEmail() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.email = ""
	s.emailErr = auth.ErrNoEmail
	return nil
}

func (s *fakeAuthStore) Keys() ([]string, error) {
	return nil, nil
}

func withFakeAuthStore(t *testing.T, store auth.Store) *atomic.Int32 {
	return withFakeAuthStoreDelay(t, store, 0)
}

func withFakeAuthStoreDelay(t *testing.T, store auth.Store, openDelay time.Duration) *atomic.Int32 {
	t.Helper()
	old := openAuthStore
	openCalls := &atomic.Int32{}
	openAuthStore = func() (auth.Store, error) {
		openCalls.Add(1)
		if openDelay > 0 {
			time.Sleep(openDelay)
		}
		return store, nil
	}
	t.Cleanup(func() {
		openAuthStore = old
	})
	return openCalls
}

func testAuthContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, &RootFlags{
		APIURL:  "https://api.example.test",
		Timeout: 30 * time.Second,
	})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})
	ctx = output.WithMode(ctx, output.Mode{})
	ctx = context.WithValue(ctx, authResolverKey{}, newAuthCredentialResolver())
	return ctx
}

func TestResolveAuthTokenPrefersEnvToken(t *testing.T) {
	t.Setenv("NIMBU_TOKEN", "env-token")

	store := &fakeAuthStore{}
	openCalls := withFakeAuthStore(t, store)

	token, err := ResolveAuthToken(context.Background())
	if err != nil {
		t.Fatalf("ResolveAuthToken: %v", err)
	}
	if token != "env-token" {
		t.Fatalf("token = %q, want env-token", token)
	}
	if openCalls.Load() != 0 {
		t.Fatalf("open store calls = %d, want 0", openCalls.Load())
	}
}

func TestResolveAuthTokenUsesCombinedCredentialAndCaches(t *testing.T) {
	store := &fakeAuthStore{
		credential: auth.Credential{Token: "cred-token", Email: "me@example.com"},
	}
	openCalls := withFakeAuthStore(t, store)
	ctx := testAuthContext()

	token, err := ResolveAuthToken(ctx)
	if err != nil {
		t.Fatalf("ResolveAuthToken: %v", err)
	}
	if token != "cred-token" {
		t.Fatalf("token = %q, want cred-token", token)
	}

	token, err = ResolveAuthToken(ctx)
	if err != nil {
		t.Fatalf("ResolveAuthToken second call: %v", err)
	}
	if token != "cred-token" {
		t.Fatalf("token second call = %q, want cred-token", token)
	}

	if openCalls.Load() != 1 {
		t.Fatalf("open store calls = %d, want 1", openCalls.Load())
	}
	if store.getCredentialCalls != 1 {
		t.Fatalf("get credential calls = %d, want 1", store.getCredentialCalls)
	}
	if store.getTokenCalls != 0 {
		t.Fatalf("get token calls = %d, want 0", store.getTokenCalls)
	}
}

func TestResolveAuthCredentialFallsBackLegacyAndBackfills(t *testing.T) {
	store := &fakeAuthStore{
		credentialErr: auth.ErrNoToken,
		token:         "legacy-token",
		email:         "Me@Example.com",
	}
	openCalls := withFakeAuthStore(t, store)
	ctx := testAuthContext()

	cred, err := ResolveAuthCredential(ctx)
	if err != nil {
		t.Fatalf("ResolveAuthCredential: %v", err)
	}
	if cred.Token != "legacy-token" {
		t.Fatalf("token = %q, want legacy-token", cred.Token)
	}
	if cred.Email != "me@example.com" {
		t.Fatalf("email = %q, want me@example.com", cred.Email)
	}

	cred, err = ResolveAuthCredential(ctx)
	if err != nil {
		t.Fatalf("ResolveAuthCredential second call: %v", err)
	}
	if cred.Token != "legacy-token" {
		t.Fatalf("second token = %q, want legacy-token", cred.Token)
	}

	if openCalls.Load() != 1 {
		t.Fatalf("open store calls = %d, want 1", openCalls.Load())
	}
	if store.getCredentialCalls != 1 {
		t.Fatalf("get credential calls = %d, want 1", store.getCredentialCalls)
	}
	if store.getTokenCalls != 0 {
		t.Fatalf("get token calls = %d, want 0", store.getTokenCalls)
	}
	if store.getEmailCalls != 0 {
		t.Fatalf("get email calls = %d, want 0", store.getEmailCalls)
	}
	if store.setCredentialCalls != 1 {
		t.Fatalf("set credential calls = %d, want 1", store.setCredentialCalls)
	}
	if store.lastCredential.Email != "me@example.com" {
		t.Fatalf("backfilled email = %q, want me@example.com", store.lastCredential.Email)
	}
}

func TestGetAPIClientUsesCachedTokenResolver(t *testing.T) {
	store := &fakeAuthStore{
		credential: auth.Credential{Token: "cached-token"},
	}
	openCalls := withFakeAuthStore(t, store)
	ctx := testAuthContext()

	client, err := GetAPIClient(ctx)
	if err != nil {
		t.Fatalf("GetAPIClient: %v", err)
	}
	if client.Token != "cached-token" {
		t.Fatalf("client token = %q, want cached-token", client.Token)
	}

	client, err = GetAPIClient(ctx)
	if err != nil {
		t.Fatalf("GetAPIClient second call: %v", err)
	}
	if client.Token != "cached-token" {
		t.Fatalf("client token second call = %q, want cached-token", client.Token)
	}

	if openCalls.Load() != 1 {
		t.Fatalf("open store calls = %d, want 1", openCalls.Load())
	}
	if store.getCredentialCalls != 1 {
		t.Fatalf("get credential calls = %d, want 1", store.getCredentialCalls)
	}
}

func TestResolveAuthTokenConcurrentUsesSingleLoad(t *testing.T) {
	store := &fakeAuthStore{
		credential:      auth.Credential{Token: "cred-token", Email: "me@example.com"},
		credentialDelay: 20 * time.Millisecond,
	}
	openCalls := withFakeAuthStoreDelay(t, store, 20*time.Millisecond)
	ctx := testAuthContext()

	const callers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, callers)
	tokenCh := make(chan string, callers)

	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := ResolveAuthToken(ctx)
			if err != nil {
				errCh <- err
				return
			}
			tokenCh <- token
		}()
	}

	wg.Wait()
	close(errCh)
	close(tokenCh)

	for err := range errCh {
		t.Fatalf("ResolveAuthToken: %v", err)
	}
	for token := range tokenCh {
		if token != "cred-token" {
			t.Fatalf("token = %q, want cred-token", token)
		}
	}
	if openCalls.Load() != 1 {
		t.Fatalf("open store calls = %d, want 1", openCalls.Load())
	}
	if store.getCredentialCalls != 1 {
		t.Fatalf("get credential calls = %d, want 1", store.getCredentialCalls)
	}
}

func TestResolveAuthCredentialConcurrentUsesSingleLoad(t *testing.T) {
	store := &fakeAuthStore{
		credential:      auth.Credential{Token: "cred-token", Email: "me@example.com"},
		credentialDelay: 20 * time.Millisecond,
	}
	openCalls := withFakeAuthStoreDelay(t, store, 20*time.Millisecond)
	ctx := testAuthContext()

	const callers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, callers)
	credCh := make(chan auth.Credential, callers)

	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cred, err := ResolveAuthCredential(ctx)
			if err != nil {
				errCh <- err
				return
			}
			credCh <- cred
		}()
	}

	wg.Wait()
	close(errCh)
	close(credCh)

	for err := range errCh {
		t.Fatalf("ResolveAuthCredential: %v", err)
	}
	for cred := range credCh {
		if cred.Token != "cred-token" || cred.Email != "me@example.com" {
			t.Fatalf("credential = %#v", cred)
		}
	}
	if openCalls.Load() != 1 {
		t.Fatalf("open store calls = %d, want 1", openCalls.Load())
	}
	if store.getCredentialCalls != 1 {
		t.Fatalf("get credential calls = %d, want 1", store.getCredentialCalls)
	}
}

func TestDeleteStoredCredentialsClearsCachedCredential(t *testing.T) {
	store := &fakeAuthStore{
		credential: auth.Credential{Token: "cached-token"},
	}
	withFakeAuthStore(t, store)
	ctx := testAuthContext()

	if _, err := ResolveAuthToken(ctx); err != nil {
		t.Fatalf("ResolveAuthToken: %v", err)
	}
	if err := DeleteStoredCredentials(ctx); err != nil {
		t.Fatalf("DeleteStoredCredentials: %v", err)
	}
	if _, err := ResolveAuthToken(ctx); !errors.Is(err, auth.ErrNoToken) {
		t.Fatalf("ResolveAuthToken after delete err = %v, want ErrNoToken", err)
	}
	if store.deleteCredentialCalls != 1 {
		t.Fatalf("delete credential calls = %d, want 1", store.deleteCredentialCalls)
	}
}

func TestResolveAuthCredentialNoTokenAlsoCachesTokenMiss(t *testing.T) {
	store := &fakeAuthStore{
		credentialErr: auth.ErrNoToken,
		tokenErr:      auth.ErrNoToken,
		emailErr:      auth.ErrNoEmail,
	}
	openCalls := withFakeAuthStore(t, store)
	ctx := testAuthContext()

	if _, err := ResolveAuthCredential(ctx); !errors.Is(err, auth.ErrNoToken) {
		t.Fatalf("ResolveAuthCredential err = %v, want ErrNoToken", err)
	}
	if _, err := ResolveAuthToken(ctx); !errors.Is(err, auth.ErrNoToken) {
		t.Fatalf("ResolveAuthToken err = %v, want ErrNoToken", err)
	}

	if openCalls.Load() != 1 {
		t.Fatalf("open store calls = %d, want 1", openCalls.Load())
	}
	if store.getCredentialCalls != 1 {
		t.Fatalf("get credential calls = %d, want 1", store.getCredentialCalls)
	}
	if store.getTokenCalls != 0 {
		t.Fatalf("get token calls = %d, want 0", store.getTokenCalls)
	}
}

func TestAuthLoginStoreTokenWritesCombinedCredential(t *testing.T) {
	store := &fakeAuthStore{}
	withFakeAuthStore(t, store)

	cmd := &AuthLoginCmd{}
	if err := cmd.storeToken(context.Background(), "tok", "me@example.com"); err != nil {
		t.Fatalf("storeToken: %v", err)
	}

	if store.setCredentialCalls != 1 {
		t.Fatalf("set credential calls = %d, want 1", store.setCredentialCalls)
	}
	if store.setTokenCalls != 0 {
		t.Fatalf("set token calls = %d, want 0", store.setTokenCalls)
	}
	if store.setEmailCalls != 0 {
		t.Fatalf("set email calls = %d, want 0", store.setEmailCalls)
	}
	if store.lastCredential.Token != "tok" || store.lastCredential.Email != "me@example.com" {
		t.Fatalf("stored credential = %#v", store.lastCredential)
	}
}

func TestAuthLogoutUsesCachedStoreOnce(t *testing.T) {
	store := &fakeAuthStore{
		credential: auth.Credential{Token: "logout-token"},
	}
	openCalls := withFakeAuthStore(t, store)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/logout" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := testAuthContext()
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	flags.APIURL = srv.URL

	out := captureStdout(t, func() error {
		return (&AuthLogoutCmd{}).Run(ctx)
	})
	if !strings.Contains(out, "Logged out") {
		t.Fatalf("stdout = %q, want logout message", out)
	}
	if openCalls.Load() != 1 {
		t.Fatalf("open store calls = %d, want 1", openCalls.Load())
	}
	if store.getCredentialCalls != 1 {
		t.Fatalf("get credential calls = %d, want 1", store.getCredentialCalls)
	}
	if store.deleteCredentialCalls != 1 {
		t.Fatalf("delete credential calls = %d, want 1", store.deleteCredentialCalls)
	}
}
