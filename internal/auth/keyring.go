package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/99designs/keyring"
	"golang.org/x/term"

	"github.com/nimbu/cli/internal/config"
)

// Store provides credential storage operations.
type Store interface {
	SetToken(token string) error
	GetToken() (string, error)
	DeleteToken() error
	SetEmail(email string) error
	GetEmail() (string, error)
	DeleteEmail() error
	Keys() ([]string, error)
}

// KeyringStore implements Store using OS keychain.
type KeyringStore struct {
	ring keyring.Keyring
}

// Credential holds stored authentication data.
type Credential struct {
	Token     string    `json:"token"`
	Email     string    `json:"email,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

const (
	keyringPasswordEnv = "NIMBU_KEYRING_PASSWORD"
	keyringBackendEnv  = "NIMBU_KEYRING_BACKEND"
	tokenKey           = "token"
	emailKey           = "email"
	credentialKey      = "credential"
)

var (
	ErrNoToken        = errors.New("no token stored")
	ErrNoEmail        = errors.New("no email stored")
	ErrInvalidBackend = errors.New("invalid keyring backend")
	ErrKeyringTimeout = errors.New("keyring connection timed out")
	errNoTTY          = errors.New("no TTY available for keyring file backend password prompt")
)

// OpenDefault opens the default keyring store.
func OpenDefault() (Store, error) {
	ring, err := openKeyring()
	if err != nil {
		return nil, err
	}
	return &KeyringStore{ring: ring}, nil
}

// ResolveBackend determines which keyring backend to use.
func ResolveBackend() (string, string, error) {
	// Check environment variable
	if v := normalizeBackend(os.Getenv(keyringBackendEnv)); v != "" {
		return v, "env", nil
	}

	// Check config file
	cfg, err := config.Read()
	if err == nil && cfg.KeyringBackend != "" {
		if v := normalizeBackend(cfg.KeyringBackend); v != "" {
			return v, "config", nil
		}
	}

	return "auto", "default", nil
}

func normalizeBackend(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func allowedBackends(backend string) ([]keyring.BackendType, error) {
	switch backend {
	case "", "auto":
		return nil, nil // Use default backend selection
	case "keychain":
		return []keyring.BackendType{keyring.KeychainBackend}, nil
	case "file":
		return []keyring.BackendType{keyring.FileBackend}, nil
	case "secret-service":
		return []keyring.BackendType{keyring.SecretServiceBackend}, nil
	case "kwallet":
		return []keyring.BackendType{keyring.KWalletBackend}, nil
	case "wincred":
		return []keyring.BackendType{keyring.WinCredBackend}, nil
	default:
		return nil, fmt.Errorf("%w: %q (expected auto, keychain, file, secret-service, kwallet, or wincred)", ErrInvalidBackend, backend)
	}
}

func fileKeyringPasswordFunc() keyring.PromptFunc {
	password := os.Getenv(keyringPasswordEnv)
	if password != "" {
		return keyring.FixedStringPrompt(password)
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		return keyring.TerminalPrompt
	}

	return func(_ string) (string, error) {
		return "", fmt.Errorf("%w; set %s", errNoTTY, keyringPasswordEnv)
	}
}

const keyringOpenTimeout = 5 * time.Second

func openKeyring() (keyring.Keyring, error) {
	dataDir, err := config.EnsureDataDir()
	if err != nil {
		return nil, fmt.Errorf("ensure data dir: %w", err)
	}

	backend, _, err := ResolveBackend()
	if err != nil {
		return nil, err
	}

	backends, err := allowedBackends(backend)
	if err != nil {
		return nil, err
	}

	// On Linux with "auto" backend and no D-Bus session, force file backend
	dbusAddr := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if runtime.GOOS == "linux" && backend == "auto" && dbusAddr == "" {
		backends = []keyring.BackendType{keyring.FileBackend}
	}

	cfg := keyring.Config{
		ServiceName:              config.AppName,
		KeychainTrustApplication: false, // Support Homebrew upgrades
		AllowedBackends:          backends,
		FileDir:                  dataDir,
		FilePasswordFunc:         fileKeyringPasswordFunc(),
	}

	// On Linux with D-Bus, use timeout to prevent hanging
	if runtime.GOOS == "linux" && backend == "auto" && dbusAddr != "" {
		return openKeyringWithTimeout(cfg, keyringOpenTimeout)
	}

	ring, err := keyring.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("open keyring: %w", err)
	}

	return ring, nil
}

func openKeyringWithTimeout(cfg keyring.Config, timeout time.Duration) (keyring.Keyring, error) {
	type result struct {
		ring keyring.Keyring
		err  error
	}

	ch := make(chan result, 1)
	go func() {
		ring, err := keyring.Open(cfg)
		ch <- result{ring, err}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("open keyring: %w", res.err)
		}
		return res.ring, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("%w after %v; set %s=file and %s=<password> to use encrypted file storage",
			ErrKeyringTimeout, timeout, keyringBackendEnv, keyringPasswordEnv)
	}
}

// SetToken stores an authentication token.
func (s *KeyringStore) SetToken(token string) error {
	if err := s.ring.Set(keyring.Item{
		Key:  tokenKey,
		Data: []byte(token),
	}); err != nil {
		return fmt.Errorf("store token: %w", err)
	}
	return nil
}

// GetToken retrieves the stored authentication token.
func (s *KeyringStore) GetToken() (string, error) {
	item, err := s.ring.Get(tokenKey)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return "", ErrNoToken
		}
		return "", fmt.Errorf("read token: %w", err)
	}
	return string(item.Data), nil
}

// DeleteToken removes the stored token.
func (s *KeyringStore) DeleteToken() error {
	if err := s.ring.Remove(tokenKey); err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return nil // Already deleted
		}
		return fmt.Errorf("delete token: %w", err)
	}
	return nil
}

// SetEmail stores the user's email.
func (s *KeyringStore) SetEmail(email string) error {
	if err := s.ring.Set(keyring.Item{
		Key:  emailKey,
		Data: []byte(strings.ToLower(strings.TrimSpace(email))),
	}); err != nil {
		return fmt.Errorf("store email: %w", err)
	}
	return nil
}

// GetEmail retrieves the stored email.
func (s *KeyringStore) GetEmail() (string, error) {
	item, err := s.ring.Get(emailKey)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return "", ErrNoEmail
		}
		return "", fmt.Errorf("read email: %w", err)
	}
	return string(item.Data), nil
}

// DeleteEmail removes the stored email.
func (s *KeyringStore) DeleteEmail() error {
	if err := s.ring.Remove(emailKey); err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("delete email: %w", err)
	}
	return nil
}

// SetCredential stores a full credential (token + email + metadata).
func (s *KeyringStore) SetCredential(cred Credential) error {
	if cred.CreatedAt.IsZero() {
		cred.CreatedAt = time.Now().UTC()
	}

	data, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("encode credential: %w", err)
	}

	if err := s.ring.Set(keyring.Item{
		Key:  credentialKey,
		Data: data,
	}); err != nil {
		return fmt.Errorf("store credential: %w", err)
	}

	return nil
}

// GetCredential retrieves the full credential.
func (s *KeyringStore) GetCredential() (Credential, error) {
	item, err := s.ring.Get(credentialKey)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			// Fall back to separate token/email
			token, tokenErr := s.GetToken()
			if tokenErr != nil {
				return Credential{}, ErrNoToken
			}
			email, _ := s.GetEmail()
			return Credential{Token: token, Email: email}, nil
		}
		return Credential{}, fmt.Errorf("read credential: %w", err)
	}

	var cred Credential
	if err := json.Unmarshal(item.Data, &cred); err != nil {
		return Credential{}, fmt.Errorf("decode credential: %w", err)
	}

	return cred, nil
}

// DeleteCredential removes all stored credentials.
func (s *KeyringStore) DeleteCredential() error {
	_ = s.ring.Remove(credentialKey)
	_ = s.ring.Remove(tokenKey)
	_ = s.ring.Remove(emailKey)
	return nil
}

// Keys returns all stored keys.
func (s *KeyringStore) Keys() ([]string, error) {
	keys, err := s.ring.Keys()
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}
	return keys, nil
}

// HasToken checks if a token is stored.
func (s *KeyringStore) HasToken() bool {
	_, err := s.GetToken()
	return err == nil
}
