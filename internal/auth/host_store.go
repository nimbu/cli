package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/99designs/keyring"

	"github.com/nimbu/cli/internal/config"
)

// HostStore wraps a keyring.Keyring and keys credentials by API host,
// so different API endpoints have separate credential storage.
type HostStore struct {
	ring keyring.Keyring
	host string
}

// NewHostStore creates a host-scoped credential store.
func NewHostStore(ring keyring.Keyring, host string) *HostStore {
	return &HostStore{ring: ring, host: host}
}

func (s *HostStore) hostKey(base string) string {
	return base + ":" + s.host
}

func (s *HostStore) hostLabel(base string) string {
	return base + " (" + s.host + ")"
}

func (s *HostStore) isDefaultHost() bool {
	return s.host == config.DefaultAPIHost
}

// SetCredential stores a credential under the host-scoped key.
func (s *HostStore) SetCredential(cred Credential) error {
	cred.Token = strings.TrimSpace(cred.Token)
	cred.Email = strings.ToLower(strings.TrimSpace(cred.Email))
	if cred.Token == "" {
		return fmt.Errorf("store credential: token required")
	}
	if cred.CreatedAt.IsZero() {
		cred.CreatedAt = time.Now().UTC()
	}

	data, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("encode credential: %w", err)
	}

	if err := s.ring.Set(keyring.Item{
		Key:   s.hostKey(credentialKey),
		Data:  data,
		Label: s.hostLabel("Nimbu CLI credential"),
	}); err != nil {
		return fmt.Errorf("store credential: %w", err)
	}

	return nil
}

// GetCredential retrieves the credential for this host.
// For the default host, falls back to bare legacy keys and backfills.
func (s *HostStore) GetCredential() (Credential, error) {
	// Try host-scoped key first
	item, err := s.ring.Get(s.hostKey(credentialKey))
	if err == nil {
		return decodeCredential(item.Data)
	}
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		return Credential{}, fmt.Errorf("read credential: %w", err)
	}

	// Only fall back to legacy bare keys for the default host
	if !s.isDefaultHost() {
		return Credential{}, ErrNoToken
	}

	return s.readLegacyAndBackfill()
}

// DeleteCredential removes credentials for this host.
// For the default host, also removes bare legacy keys to prevent
// the legacy fallback from resurrecting deleted credentials.
func (s *HostStore) DeleteCredential() error {
	_ = s.ring.Remove(s.hostKey(credentialKey))
	_ = s.ring.Remove(s.hostKey(tokenKey))
	_ = s.ring.Remove(s.hostKey(emailKey))
	if s.isDefaultHost() {
		_ = s.ring.Remove(credentialKey)
		_ = s.ring.Remove(tokenKey)
		_ = s.ring.Remove(emailKey)
	}
	return nil
}

func (s *HostStore) SetToken(token string) error {
	if err := s.ring.Set(keyring.Item{
		Key:   s.hostKey(tokenKey),
		Data:  []byte(token),
		Label: s.hostLabel("Nimbu CLI token"),
	}); err != nil {
		return fmt.Errorf("store token: %w", err)
	}
	return nil
}

func (s *HostStore) GetToken() (string, error) {
	item, err := s.ring.Get(s.hostKey(tokenKey))
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return "", ErrNoToken
		}
		return "", fmt.Errorf("read token: %w", err)
	}
	return string(item.Data), nil
}

func (s *HostStore) DeleteToken() error {
	if err := s.ring.Remove(s.hostKey(tokenKey)); err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("delete token: %w", err)
	}
	return nil
}

func (s *HostStore) SetEmail(email string) error {
	if err := s.ring.Set(keyring.Item{
		Key:   s.hostKey(emailKey),
		Data:  []byte(strings.ToLower(strings.TrimSpace(email))),
		Label: s.hostLabel("Nimbu CLI email"),
	}); err != nil {
		return fmt.Errorf("store email: %w", err)
	}
	return nil
}

func (s *HostStore) GetEmail() (string, error) {
	item, err := s.ring.Get(s.hostKey(emailKey))
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return "", ErrNoEmail
		}
		return "", fmt.Errorf("read email: %w", err)
	}
	return string(item.Data), nil
}

func (s *HostStore) DeleteEmail() error {
	if err := s.ring.Remove(s.hostKey(emailKey)); err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("delete email: %w", err)
	}
	return nil
}

func (s *HostStore) Keys() ([]string, error) {
	keys, err := s.ring.Keys()
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}
	return keys, nil
}

// readLegacyAndBackfill reads from bare legacy keys and backfills to host-scoped storage.
func (s *HostStore) readLegacyAndBackfill() (Credential, error) {
	// Try bare combined credential key first
	item, err := s.ring.Get(credentialKey)
	if err == nil {
		cred, err := decodeCredential(item.Data)
		if err == nil {
			_ = s.SetCredential(cred)
			return cred, nil
		}
	}

	// Fall back to bare individual token + email keys
	tokenItem, err := s.ring.Get(tokenKey)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return Credential{}, ErrNoToken
		}
		return Credential{}, fmt.Errorf("read token: %w", err)
	}
	token := strings.TrimSpace(string(tokenItem.Data))
	if token == "" {
		return Credential{}, ErrNoToken
	}

	var email string
	emailItem, err := s.ring.Get(emailKey)
	if err == nil {
		email = strings.ToLower(strings.TrimSpace(string(emailItem.Data)))
	}

	cred := Credential{Token: token, Email: email}
	_ = s.SetCredential(cred)
	return cred, nil
}
