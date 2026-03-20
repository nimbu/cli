package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/config"
)

type authResolverKey struct{}

var openAuthStore = auth.OpenForHost

type authCredentialResolver struct {
	mu sync.Mutex

	host      string
	openStore func(host string) (auth.Store, error)

	store     auth.Store
	storeErr  error
	storeOpen bool

	token         string
	tokenErr      error
	tokenLoaded   bool
	credential    auth.Credential
	credentialErr error
	credLoaded    bool
}

func newAuthCredentialResolver(host string) *authCredentialResolver {
	return &authCredentialResolver{host: host, openStore: openAuthStore}
}

func resolverFromContext(ctx context.Context) *authCredentialResolver {
	if ctx != nil {
		if resolver, ok := ctx.Value(authResolverKey{}).(*authCredentialResolver); ok && resolver != nil {
			return resolver
		}
	}
	return newAuthCredentialResolver(config.DefaultAPIHost)
}

func ResolveAuthToken(ctx context.Context) (string, error) {
	if token := strings.TrimSpace(os.Getenv("NIMBU_TOKEN")); token != "" {
		return token, nil
	}
	return resolverFromContext(ctx).Token()
}

func ResolveAuthCredential(ctx context.Context) (auth.Credential, error) {
	if token := strings.TrimSpace(os.Getenv("NIMBU_TOKEN")); token != "" {
		return auth.Credential{Token: token}, nil
	}
	return resolverFromContext(ctx).Credential()
}

func DeleteStoredCredentials(ctx context.Context) error {
	return resolverFromContext(ctx).DeleteStoredCredentials()
}

func (r *authCredentialResolver) Token() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tokenLoaded {
		return r.token, r.tokenErr
	}

	store, err := r.storeInstanceLocked()
	if err != nil {
		return r.cacheTokenLocked("", fmt.Errorf("open keyring: %w", err))
	}

	cred, err := store.GetCredential()
	switch {
	case err == nil:
		cred, err = r.cacheCredentialLocked(cred)
		return cred.Token, err
	case !errors.Is(err, auth.ErrNoToken):
		return r.cacheTokenLocked("", fmt.Errorf("get credential: %w", err))
	}

	token, err := store.GetToken()
	if err != nil {
		if errors.Is(err, auth.ErrNoToken) {
			return r.cacheTokenLocked("", auth.ErrNoToken)
		}
		return r.cacheTokenLocked("", fmt.Errorf("get token: %w", err))
	}
	return r.cacheTokenLocked(strings.TrimSpace(token), nil)
}

func (r *authCredentialResolver) Credential() (auth.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.credLoaded {
		return r.credential, r.credentialErr
	}

	store, err := r.storeInstanceLocked()
	if err != nil {
		return r.cacheCredentialValueLocked(auth.Credential{}, fmt.Errorf("open keyring: %w", err))
	}

	cred, err := store.GetCredential()
	switch {
	case err == nil:
		return r.cacheCredentialLocked(cred)
	case !errors.Is(err, auth.ErrNoToken):
		return r.cacheCredentialValueLocked(auth.Credential{}, fmt.Errorf("get credential: %w", err))
	}
	return r.cacheCredentialValueLocked(auth.Credential{}, auth.ErrNoToken)
}

func (r *authCredentialResolver) DeleteStoredCredentials() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	store, err := r.storeInstanceLocked()
	if err != nil {
		return fmt.Errorf("open keyring: %w", err)
	}
	if err := store.DeleteCredential(); err != nil {
		return fmt.Errorf("delete credentials: %w", err)
	}
	r.token = ""
	r.tokenErr = auth.ErrNoToken
	r.tokenLoaded = true
	r.credential = auth.Credential{}
	r.credentialErr = auth.ErrNoToken
	r.credLoaded = true
	return nil
}

func (r *authCredentialResolver) storeInstanceLocked() (auth.Store, error) {
	if !r.storeOpen {
		r.store, r.storeErr = r.openStore(r.host)
		r.storeOpen = true
	}
	return r.store, r.storeErr
}

func (r *authCredentialResolver) cacheCredentialLocked(cred auth.Credential) (auth.Credential, error) {
	cred.Token = strings.TrimSpace(cred.Token)
	cred.Email = strings.ToLower(strings.TrimSpace(cred.Email))
	return r.cacheCredentialValueLocked(cred, nil)
}

func (r *authCredentialResolver) cacheCredentialValueLocked(cred auth.Credential, err error) (auth.Credential, error) {
	r.credential = cred
	r.credentialErr = err
	r.credLoaded = true
	if err == nil {
		r.token = cred.Token
		r.tokenErr = nil
		r.tokenLoaded = true
	} else if !r.tokenLoaded {
		r.token = ""
		r.tokenErr = err
		r.tokenLoaded = true
	}
	return cred, err
}

func (r *authCredentialResolver) cacheTokenLocked(token string, err error) (string, error) {
	token = strings.TrimSpace(token)
	r.token = token
	r.tokenErr = err
	r.tokenLoaded = true
	if err != nil && !r.credLoaded {
		r.credential = auth.Credential{}
		r.credentialErr = err
		r.credLoaded = true
	}
	return token, err
}
