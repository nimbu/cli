package oauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"

	"github.com/nimbu/cli/internal/auth"
)

// RefreshMargin is how long before expiry an access token is renewed.
const RefreshMargin = 60 * time.Second

// lockWait bounds how long a refresh waits for another process's refresh.
const lockWait = 30 * time.Second

// Store persists the session. GetCredential must read the backing store
// rather than a cache: a refresh re-reads it to pick up a rotation another
// process already made.
type Store interface {
	GetCredential() (auth.Credential, error)
	SetCredential(cred auth.Credential) error
	DeleteCredential() error
}

// TokenSource hands out the access token of a stored OAuth session and
// renews it with the rotating refresh token. It is safe for concurrent use:
// refreshes are single-flight within the process and serialised across
// processes with a lock file, because the server treats a reused refresh
// token as theft and revokes the whole session.
type TokenSource struct {
	client   *Client
	store    Store
	lockPath string
	now      func() time.Time

	mu   sync.Mutex
	cred auth.Credential
}

// NewTokenSource returns a token source for cred. lockPath may be empty to
// skip cross-process locking.
func NewTokenSource(client *Client, store Store, cred auth.Credential, lockPath string) *TokenSource {
	if cred.ClientID != "" {
		client.ClientID = cred.ClientID
	}
	return &TokenSource{client: client, store: store, cred: cred, lockPath: lockPath, now: time.Now}
}

// LockPath returns the refresh lock file for host inside dir.
func LockPath(dir, host string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			return r
		default:
			return '_'
		}
	}, host)
	return filepath.Join(dir, "oauth-refresh-"+safe+".lock")
}

// Token returns a usable access token, refreshing it when it is about to expire.
func (s *TokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fresh(s.cred) {
		return s.cred.Token, nil
	}
	current := s.cred
	token, err := s.refreshLocked(ctx, current.Token)
	if err != nil && !errors.Is(err, ErrSessionExpired) && current.Token != "" && s.now().Before(current.ExpiresAt) {
		// Refresh failed transiently inside the margin; the token still works.
		return current.Token, nil
	}
	return token, err
}

// Renew replaces an access token the API rejected. Concurrent callers that
// saw the same rejected token share one refresh.
func (s *TokenSource) Renew(ctx context.Context, rejected string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cred.Token != "" && s.cred.Token != rejected {
		return s.cred.Token, nil
	}
	return s.refreshLocked(ctx, rejected)
}

func (s *TokenSource) fresh(cred auth.Credential) bool {
	if cred.Token == "" {
		return false
	}
	if !cred.IsOAuth() || cred.ExpiresAt.IsZero() {
		return true
	}
	return s.now().Add(RefreshMargin).Before(cred.ExpiresAt)
}

func (s *TokenSource) refreshLocked(ctx context.Context, stale string) (string, error) {
	unlock, err := s.lockFile(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()

	// Another process may have rotated the session while we waited.
	stored, err := s.store.GetCredential()
	switch {
	case errors.Is(err, auth.ErrNoToken):
		s.cred = auth.Credential{}
		return "", ErrSessionExpired
	case err == nil && stored.Token != s.cred.Token:
		s.cred = stored
		if stored.Token != stale && s.fresh(stored) {
			return stored.Token, nil
		}
	}

	used := s.cred.RefreshToken
	if used == "" {
		return "", ErrSessionExpired
	}
	tok, err := s.client.Refresh(ctx, used)
	if err != nil && !IsInvalidGrant(err) {
		return "", refreshFailure(err)
	}

	// Only replace or delete the session this refresh started from. A login
	// or logout that could not take the lock, or a keyring read that failed
	// above, may have changed the store during the request.
	current, readErr := s.store.GetCredential()
	switch {
	case errors.Is(readErr, auth.ErrNoToken):
		s.cred = auth.Credential{}
		return "", ErrSessionExpired
	case readErr == nil && current.RefreshToken != used:
		s.cred = current
		return current.Token, nil
	}

	if err != nil {
		// invalid_grant: revoked, expired or rotated away, so the session is
		// gone. Delete it only when the store verifiably still holds it.
		if readErr == nil {
			_ = s.store.DeleteCredential()
		}
		s.cred = auth.Credential{}
		return "", ErrSessionExpired
	}

	next := Credential(tok, s.client.ClientID)
	next.Email = s.cred.Email
	next.CreatedAt = s.cred.CreatedAt
	if len(next.Scopes) == 0 {
		next.Scopes = s.cred.Scopes
	}
	s.cred = next
	if err := s.store.SetCredential(next); err != nil {
		// The old refresh token is already dead; this process can go on,
		// the next one will have to log in again.
		slog.Warn("could not save the refreshed session; run `nimbu auth login` if later commands fail", "error", err)
	}
	return next.Token, nil
}

func (s *TokenSource) lockFile(ctx context.Context) (func(), error) {
	unlock, err := LockSession(ctx, s.lockPath)
	if err != nil && ctx.Err() == nil {
		return nil, fmt.Errorf("refresh session: %w", err)
	}
	return unlock, err
}

// LockSession takes the cross-process lock that guards the session stored
// for one host; the returned func releases it. Anything that replaces or
// deletes that session (refresh, login, logout) holds it, so a refresh never
// writes back over a newer login. An empty lockPath, or a lock file that
// cannot be used, skips the lock rather than blocking the command.
func LockSession(ctx context.Context, lockPath string) (func(), error) {
	noop := func() {}
	if lockPath == "" {
		return noop, nil
	}
	lock := flock.New(lockPath)
	waitCtx, cancel := context.WithTimeout(ctx, lockWait)
	defer cancel()
	locked, err := lock.TryLockContext(waitCtx, 50*time.Millisecond)
	if err != nil && ctx.Err() == nil && !errors.Is(err, context.DeadlineExceeded) {
		// The lock file is unusable (permissions, read-only disk): go on
		// without cross-process protection rather than not at all.
		slog.Debug("session lock unavailable", "path", lockPath, "error", err)
		return noop, nil
	}
	if err != nil || !locked {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("timed out waiting for another nimbu process to update the session")
	}
	return func() { _ = lock.Unlock() }, nil
}
