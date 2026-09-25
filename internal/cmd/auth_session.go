package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/oauth"
)

// envToken returns NIMBU_TOKEN. It always wins over stored credentials and is
// never refreshed or persisted.
func envToken() string {
	return strings.TrimSpace(os.Getenv("NIMBU_TOKEN"))
}

// resolverForBaseURL returns the credential resolver for the API at baseURL;
// the session's own --apiurl always maps to the session resolver.
func resolverForBaseURL(ctx context.Context, baseURL string) *authCredentialResolver {
	if flags, ok := ctx.Value(rootFlagsKey{}).(*RootFlags); ok && flags != nil &&
		strings.TrimRight(strings.TrimSpace(baseURL), "/") == strings.TrimRight(strings.TrimSpace(flags.APIURL), "/") {
		return resolverFromContext(ctx)
	}
	return resolverForHost(ctx, apps.NormalizeHost(baseURL))
}

// resolverForHost returns the credential resolver for host, reusing the
// session resolver for its own host and caching the others, so every client
// for one host shares one refreshing session.
func resolverForHost(ctx context.Context, host string) *authCredentialResolver {
	root := resolverFromContext(ctx)
	if host == "" || host == root.host {
		return root
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	if root.others == nil {
		root.others = map[string]*authCredentialResolver{}
	}
	resolver, ok := root.others[host]
	if !ok {
		resolver = newAuthCredentialResolver(host)
		root.others[host] = resolver
	}
	return resolver
}

// forget drops everything cached, so the next lookup reads the keyring again.
func (r *authCredentialResolver) forget() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.token, r.tokenErr, r.tokenLoaded = "", nil, false
	r.credential, r.credentialErr, r.credLoaded = auth.Credential{}, nil, false
	r.tokens = nil
}

// newAuthenticatedClient builds an API client for baseURL that authenticates
// with the credential stored for that host. It is not readonly: callers that
// honour --readonly add it themselves.
func newAuthenticatedClient(ctx context.Context, baseURL string) (*api.Client, error) {
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	client := api.New(baseURL, "").
		WithVersion(version).
		WithTimeout(flags.Timeout).
		WithDebug(flags.Debug)
	return authenticateClient(ctx, client)
}

// authenticateClient attaches the credential stored for the client's API host:
// NIMBU_TOKEN, a refreshing OAuth session, or a static API token.
func authenticateClient(ctx context.Context, client *api.Client) (*api.Client, error) {
	if token := envToken(); token != "" {
		client.Token = token
		return client, nil
	}
	resolver := resolverForBaseURL(ctx, client.BaseURL)
	token, err := resolver.Token()
	if err != nil {
		return nil, err
	}
	session, err := resolver.session(client.BaseURL, client.HTTPClient.Timeout)
	if err != nil {
		return nil, err
	}
	if session != nil {
		return client.WithTokenSource(session), nil
	}
	client.Token = token
	return client, nil
}

// session returns the shared token source for a stored OAuth credential, or
// nil when the stored credential is a static token.
func (r *authCredentialResolver) session(apiURL string, timeout time.Duration) (*oauth.TokenSource, error) {
	cred, err := r.Credential()
	if errors.Is(err, auth.ErrNoToken) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !cred.IsOAuth() {
		return nil, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tokens == nil {
		client := oauth.NewClient(apiURL, &http.Client{Timeout: timeout})
		r.tokens = oauth.NewTokenSource(client, resolverStore{r}, cred, refreshLockPath(r.host))
	}
	return r.tokens, nil
}

// refreshLockDir holds the cross-process refresh lock files; swapped in tests.
var refreshLockDir = config.EnsureDataDir

func refreshLockPath(host string) string {
	dir, err := refreshLockDir()
	if err != nil {
		return ""
	}
	return oauth.LockPath(dir, host)
}

// resolverStore persists refreshed sessions to the keyring and keeps the
// resolver's cache in step, so later clients get the new access token.
type resolverStore struct{ r *authCredentialResolver }

func (s resolverStore) GetCredential() (auth.Credential, error) {
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	store, err := s.r.storeInstanceLocked()
	if err != nil {
		return auth.Credential{}, fmt.Errorf("open keyring: %w", err)
	}
	return store.GetCredential()
}

func (s resolverStore) SetCredential(cred auth.Credential) error {
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	store, err := s.r.storeInstanceLocked()
	if err != nil {
		return fmt.Errorf("open keyring: %w", err)
	}
	if err := store.SetCredential(cred); err != nil {
		return err
	}
	_, _ = s.r.cacheCredentialLocked(cred)
	return nil
}

func (s resolverStore) DeleteCredential() error {
	return s.r.DeleteStoredCredentials()
}
