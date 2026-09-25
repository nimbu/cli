package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/auth"
)

// memStore stands in for the keyring shared by every CLI process.
type memStore struct {
	mu      sync.Mutex
	cred    auth.Credential
	present bool
	deletes int
	getErr  error // returned by GetCredential while set
}

func newMemStore(cred auth.Credential) *memStore {
	return &memStore{cred: cred, present: true}
}

func (m *memStore) GetCredential() (auth.Credential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return auth.Credential{}, m.getErr
	}
	if !m.present {
		return auth.Credential{}, auth.ErrNoToken
	}
	return m.cred, nil
}

func (m *memStore) SetCredential(cred auth.Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cred, m.present = cred, true
	return nil
}

func (m *memStore) DeleteCredential() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cred, m.present = auth.Credential{}, false
	m.deletes++
	return nil
}

func session(access, refresh string, expiresAt time.Time) auth.Credential {
	return auth.Credential{
		Token:        access,
		AuthMethod:   auth.AuthMethodOAuth,
		RefreshToken: refresh,
		ExpiresAt:    expiresAt,
		Email:        "me@example.com",
		ClientID:     "nimbu-cli",
		Scopes:       []string{"read_channels"},
	}
}

// rotation counts what a rotatingServer saw.
type rotation struct {
	refreshes atomic.Int32 // successful rotations
	replays   atomic.Int32 // refreshes presenting an already spent token
}

// rotatingServer issues access-N/refresh-N on each refresh. Like the real
// server's reuse detection, a spent refresh token revokes the whole session:
// it and every later refresh get invalid_grant.
func rotatingServer(t *testing.T, delay time.Duration) (*fakeServer, *rotation) {
	t.Helper()
	fs := newFakeServer(t)
	var mu sync.Mutex
	current := "refresh-0"
	spent := map[string]bool{}
	counts := &rotation{}
	fs.setToken(func(form url.Values) (int, map[string]any) {
		time.Sleep(delay)
		mu.Lock()
		defer mu.Unlock()
		presented := form.Get("refresh_token")
		if spent[presented] {
			counts.replays.Add(1)
			current = ""
		}
		if presented != current || current == "" {
			return http.StatusBadRequest, map[string]any{"error": "invalid_grant"}
		}
		spent[presented] = true
		n := counts.refreshes.Add(1)
		current = "refresh-" + strconv.Itoa(int(n))
		return http.StatusOK, tokenResponse("access-"+strconv.Itoa(int(n)), current)
	})
	return fs, counts
}

func TestTokenSourceReturnsFreshTokenWithoutRefresh(t *testing.T) {
	fs, counts := rotatingServer(t, 0)
	store := newMemStore(session("access-0", "refresh-0", time.Now().Add(30*time.Minute)))
	ts := NewTokenSource(fs.client(), store, store.cred, "")

	token, err := ts.Token(context.Background())
	if err != nil || token != "access-0" {
		t.Fatalf("Token = %q, %v", token, err)
	}
	if counts.refreshes.Load() != 0 {
		t.Fatalf("refreshes = %d, want 0", counts.refreshes.Load())
	}
}

func TestTokenSourceRefreshesNearExpiryAndPersists(t *testing.T) {
	fs, counts := rotatingServer(t, 0)
	store := newMemStore(session("access-0", "refresh-0", time.Now().Add(30*time.Second)))
	ts := NewTokenSource(fs.client(), store, store.cred, "")

	token, err := ts.Token(context.Background())
	if err != nil || token != "access-1" {
		t.Fatalf("Token = %q, %v", token, err)
	}
	saved, _ := store.GetCredential()
	if counts.refreshes.Load() != 1 || saved.Token != "access-1" || saved.RefreshToken != "refresh-1" {
		t.Fatalf("refreshes = %d, saved = %+v", counts.refreshes.Load(), saved)
	}
	if saved.Email != "me@example.com" || !saved.IsOAuth() || time.Until(saved.ExpiresAt) < 29*time.Minute {
		t.Fatalf("saved credential lost session details: %+v", saved)
	}
}

func TestTokenSourceRenewIsSingleFlight(t *testing.T) {
	fs, counts := rotatingServer(t, 20*time.Millisecond)
	store := newMemStore(session("access-0", "refresh-0", time.Now().Add(30*time.Minute)))
	ts := NewTokenSource(fs.client(), store, store.cred, "")

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := ts.Renew(context.Background(), "access-0")
			if err != nil || token != "access-1" {
				t.Errorf("Renew = %q, %v", token, err)
			}
		}()
	}
	wg.Wait()
	if counts.refreshes.Load() != 1 || counts.replays.Load() != 0 {
		t.Fatalf("refreshes = %d, replays = %d; want 1 and 0", counts.refreshes.Load(), counts.replays.Load())
	}
}

func TestTokenSourceSerialisesRefreshAcrossProcesses(t *testing.T) {
	fs, counts := rotatingServer(t, 50*time.Millisecond)
	initial := session("access-0", "refresh-0", time.Now().Add(-time.Minute))
	store := newMemStore(initial)
	lockPath := filepath.Join(t.TempDir(), "refresh.lock")

	// Two token sources over one store and lock file behave like two CLI
	// processes: the second must pick up the first one's rotation instead of
	// replaying the spent refresh token.
	var wg sync.WaitGroup
	for range 2 {
		ts := NewTokenSource(fs.client(), store, initial, lockPath)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if token, err := ts.Token(context.Background()); err != nil || token != "access-1" {
				t.Errorf("Token = %q, %v", token, err)
			}
		}()
	}
	wg.Wait()
	if counts.refreshes.Load() != 1 || counts.replays.Load() != 0 {
		t.Fatalf("refreshes = %d, replays = %d; want 1 and 0", counts.refreshes.Load(), counts.replays.Load())
	}
}

func TestTokenSourceInvalidGrantLogsOut(t *testing.T) {
	fs, _ := rotatingServer(t, 0)
	store := newMemStore(session("access-0", "revoked", time.Now().Add(30*time.Minute)))
	ts := NewTokenSource(fs.client(), store, store.cred, "")

	_, err := ts.Renew(context.Background(), "access-0")
	if !errors.Is(err, ErrSessionExpired) || !errors.Is(err, auth.ErrNoToken) {
		t.Fatalf("Renew error = %v, want ErrSessionExpired", err)
	}
	if store.deletes != 1 {
		t.Fatalf("deletes = %d, want 1", store.deletes)
	}
}

func TestTokenSourceKeepsValidTokenWhenRefreshFailsTransiently(t *testing.T) {
	fs := newFakeServer(t)
	fs.setToken(func(url.Values) (int, map[string]any) {
		return http.StatusServiceUnavailable, map[string]any{"error": "temporarily_unavailable"}
	})
	store := newMemStore(session("access-0", "refresh-0", time.Now().Add(30*time.Second)))
	ts := NewTokenSource(fs.client(), store, store.cred, "")

	token, err := ts.Token(context.Background())
	if err != nil || token != "access-0" {
		t.Fatalf("Token = %q, %v; want the still-valid token", token, err)
	}
	if store.deletes != 0 {
		t.Fatal("transient failure must not log out")
	}
}

func TestTokenSourceAdoptsRotationFromAnotherProcess(t *testing.T) {
	fs, counts := rotatingServer(t, 0)
	store := newMemStore(session("access-0", "refresh-0", time.Now().Add(30*time.Minute)))
	ts := NewTokenSource(fs.client(), store, store.cred, "")

	_ = store.SetCredential(session("access-9", "refresh-9", time.Now().Add(30*time.Minute)))
	token, err := ts.Renew(context.Background(), "access-0")
	if err != nil || token != "access-9" {
		t.Fatalf("Renew = %q, %v; want the stored rotation", token, err)
	}
	if counts.refreshes.Load() != 0 {
		t.Fatalf("refreshes = %d, want 0", counts.refreshes.Load())
	}
}

func TestTokenSourceLoggedOutElsewhere(t *testing.T) {
	fs, _ := rotatingServer(t, 0)
	store := newMemStore(session("access-0", "refresh-0", time.Now().Add(30*time.Minute)))
	ts := NewTokenSource(fs.client(), store, store.cred, "")
	_ = store.DeleteCredential()

	if _, err := ts.Renew(context.Background(), "access-0"); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("Renew error = %v, want ErrSessionExpired", err)
	}
}

func TestLockPathIsFilesystemSafe(t *testing.T) {
	got := filepath.Base(LockPath(t.TempDir(), "api.example.test:8443"))
	if got != "oauth-refresh-api.example.test_8443.lock" {
		t.Fatalf("LockPath base = %q", got)
	}
}

// TestTokenSourceOnlyReplacesTheSessionItRefreshed covers a login or logout
// that lands in the store while a refresh request is in flight: the refresh
// must not write back over it or delete it.
func TestTokenSourceOnlyReplacesTheSessionItRefreshed(t *testing.T) {
	newLogin := session("access-new", "refresh-new", time.Now().Add(30*time.Minute))
	cases := []struct {
		name       string
		status     int
		body       map[string]any
		change     func(*memStore)
		wantToken  string
		wantStored *auth.Credential
	}{
		{
			name: "refresh succeeds after a new login", status: http.StatusOK, body: tokenResponse("access-1", "refresh-1"),
			change:    func(m *memStore) { _ = m.SetCredential(newLogin) },
			wantToken: "access-new", wantStored: &newLogin,
		},
		{
			name: "invalid_grant after a new login", status: http.StatusBadRequest, body: map[string]any{"error": "invalid_grant"},
			change:    func(m *memStore) { _ = m.SetCredential(newLogin) },
			wantToken: "access-new", wantStored: &newLogin,
		},
		{
			name: "refresh succeeds after a logout", status: http.StatusOK, body: tokenResponse("access-1", "refresh-1"),
			change: func(m *memStore) { _ = m.DeleteCredential() },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := session("access-0", "refresh-0", time.Now().Add(-time.Minute))
			store := newMemStore(old)
			fs := newFakeServer(t)
			fs.setToken(func(url.Values) (int, map[string]any) {
				tc.change(store)
				return tc.status, tc.body
			})
			ts := NewTokenSource(fs.client(), store, old, "")

			token, err := ts.Token(context.Background())
			if tc.wantStored == nil {
				if !errors.Is(err, ErrSessionExpired) {
					t.Fatalf("Token = %q, %v; want ErrSessionExpired", token, err)
				}
				if _, getErr := store.GetCredential(); !errors.Is(getErr, auth.ErrNoToken) {
					t.Fatal("refresh brought a logged-out session back")
				}
				return
			}
			if err != nil || token != tc.wantToken {
				t.Fatalf("Token = %q, %v; want %q", token, err, tc.wantToken)
			}
			stored, _ := store.GetCredential()
			if stored.RefreshToken != tc.wantStored.RefreshToken {
				t.Fatalf("stored = %+v, want the new login kept", stored)
			}
			if store.deletes != 0 {
				t.Fatalf("deletes = %d, want 0", store.deletes)
			}
		})
	}
}

func TestTokenSourceInvalidGrantKeepsStoreItCannotRead(t *testing.T) {
	store := newMemStore(session("access-0", "refresh-0", time.Now().Add(-time.Minute)))
	store.getErr = errors.New("keyring locked")
	fs := newFakeServer(t)
	fs.setToken(func(url.Values) (int, map[string]any) {
		return http.StatusBadRequest, map[string]any{"error": "invalid_grant"}
	})
	ts := NewTokenSource(fs.client(), store, store.cred, "")

	if _, err := ts.Token(context.Background()); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("Token error = %v, want ErrSessionExpired", err)
	}
	if store.deletes != 0 {
		t.Fatal("deleted a session it could not verify was the one that failed")
	}
}

func TestTokenSourceRefreshFailuresKeepTheSession(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     map[string]any
		want     RefreshError
		rejected bool
	}{
		{"client rejected", http.StatusUnauthorized, map[string]any{"error": "invalid_client"}, RefreshError{Status: 401, Code: "invalid_client"}, true},
		{"unauthorized client", http.StatusBadRequest, map[string]any{"error": "unauthorized_client"}, RefreshError{Status: 400, Code: "unauthorized_client"}, true},
		{"server error without a code", http.StatusBadGateway, map[string]any{"html": "<html>bad gateway</html>"}, RefreshError{Status: 502}, false},
		{"rate limited", http.StatusTooManyRequests, map[string]any{"error": "slow_down"}, RefreshError{Status: 429, Code: "slow_down"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newMemStore(session("access-0", "refresh-0", time.Now().Add(-time.Minute)))
			fs := newFakeServer(t)
			fs.setToken(func(url.Values) (int, map[string]any) { return tc.status, tc.body })
			ts := NewTokenSource(fs.client(), store, store.cred, "")

			_, err := ts.Token(context.Background())
			var re *RefreshError
			if !errors.As(err, &re) || *re != tc.want || re.Rejected() != tc.rejected {
				t.Fatalf("Token error = %#v, want %+v (rejected %v)", err, tc.want, tc.rejected)
			}
			if errors.Is(err, auth.ErrNoToken) || strings.Contains(err.Error(), "html") {
				t.Fatalf("error = %q, want no logout and no response body", err)
			}
			if store.deletes != 0 {
				t.Fatal("a refresh failure other than invalid_grant must keep the session")
			}
		})
	}
}
