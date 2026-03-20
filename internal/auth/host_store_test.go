package auth

import (
	"errors"
	"testing"

	"github.com/99designs/keyring"

	"github.com/nimbu/cli/internal/config"
)

func newTestRing(t *testing.T) keyring.Keyring {
	t.Helper()
	ring, err := keyring.Open(keyring.Config{
		AllowedBackends: []keyring.BackendType{keyring.FileBackend},
		FileDir:         t.TempDir(),
		FilePasswordFunc: func(_ string) (string, error) {
			return "test-password", nil
		},
	})
	if err != nil {
		t.Fatalf("open test keyring: %v", err)
	}
	return ring
}

func TestHostStoreIsolatesCredentialsByHost(t *testing.T) {
	ring := newTestRing(t)
	storeA := NewHostStore(ring, "api.nimbu.io")
	storeB := NewHostStore(ring, "api.nimbu.localhost")

	// Store credential under host A
	if err := storeA.SetCredential(Credential{Token: "token-a", Email: "a@example.com"}); err != nil {
		t.Fatalf("SetCredential A: %v", err)
	}

	// Store credential under host B
	if err := storeB.SetCredential(Credential{Token: "token-b", Email: "b@example.com"}); err != nil {
		t.Fatalf("SetCredential B: %v", err)
	}

	// Read back — each host sees its own credential
	credA, err := storeA.GetCredential()
	if err != nil {
		t.Fatalf("GetCredential A: %v", err)
	}
	if credA.Token != "token-a" || credA.Email != "a@example.com" {
		t.Fatalf("credential A = %#v", credA)
	}

	credB, err := storeB.GetCredential()
	if err != nil {
		t.Fatalf("GetCredential B: %v", err)
	}
	if credB.Token != "token-b" || credB.Email != "b@example.com" {
		t.Fatalf("credential B = %#v", credB)
	}
}

func TestHostStoreDeleteOnlyAffectsOwnHost(t *testing.T) {
	ring := newTestRing(t)
	storeA := NewHostStore(ring, "api.nimbu.io")
	storeB := NewHostStore(ring, "api.nimbu.localhost")

	if err := storeA.SetCredential(Credential{Token: "token-a", Email: "a@example.com"}); err != nil {
		t.Fatalf("SetCredential A: %v", err)
	}
	if err := storeB.SetCredential(Credential{Token: "token-b", Email: "b@example.com"}); err != nil {
		t.Fatalf("SetCredential B: %v", err)
	}

	// Delete host B
	if err := storeB.DeleteCredential(); err != nil {
		t.Fatalf("DeleteCredential B: %v", err)
	}

	// Host A still has its credential
	credA, err := storeA.GetCredential()
	if err != nil {
		t.Fatalf("GetCredential A after B delete: %v", err)
	}
	if credA.Token != "token-a" {
		t.Fatalf("credential A token = %q, want token-a", credA.Token)
	}

	// Host B returns ErrNoToken
	if _, err := storeB.GetCredential(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("GetCredential B after delete: got %v, want ErrNoToken", err)
	}
}

func TestHostStoreLegacyFallbackForDefaultHost(t *testing.T) {
	ring := newTestRing(t)

	// Write a bare "credential" key (simulating pre-migration state)
	bare := &KeyringStore{ring: ring}
	if err := bare.SetCredential(Credential{Token: "legacy-token", Email: "legacy@example.com"}); err != nil {
		t.Fatalf("SetCredential bare: %v", err)
	}

	// Default host should fall back to bare key
	store := NewHostStore(ring, config.DefaultAPIHost)
	cred, err := store.GetCredential()
	if err != nil {
		t.Fatalf("GetCredential default host: %v", err)
	}
	if cred.Token != "legacy-token" || cred.Email != "legacy@example.com" {
		t.Fatalf("credential = %#v", cred)
	}
}

func TestHostStoreLegacyBackfillCreatesHostKey(t *testing.T) {
	ring := newTestRing(t)

	// Write a bare "credential" key
	bare := &KeyringStore{ring: ring}
	if err := bare.SetCredential(Credential{Token: "legacy-token", Email: "legacy@example.com"}); err != nil {
		t.Fatalf("SetCredential bare: %v", err)
	}

	store := NewHostStore(ring, config.DefaultAPIHost)

	// First read triggers backfill
	if _, err := store.GetCredential(); err != nil {
		t.Fatalf("GetCredential: %v", err)
	}

	// Remove the bare key to prove the host-scoped key was written
	_ = ring.Remove(credentialKey)

	// Should still work from the host-scoped key
	cred, err := store.GetCredential()
	if err != nil {
		t.Fatalf("GetCredential after bare removal: %v", err)
	}
	if cred.Token != "legacy-token" {
		t.Fatalf("token = %q, want legacy-token", cred.Token)
	}
}

func TestHostStoreNoLegacyFallbackForNonDefaultHost(t *testing.T) {
	ring := newTestRing(t)

	// Write a bare "credential" key
	bare := &KeyringStore{ring: ring}
	if err := bare.SetCredential(Credential{Token: "legacy-token", Email: "legacy@example.com"}); err != nil {
		t.Fatalf("SetCredential bare: %v", err)
	}

	// Non-default host should NOT fall back to bare key
	store := NewHostStore(ring, "api.nimbu.localhost")
	if _, err := store.GetCredential(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("GetCredential non-default host: got %v, want ErrNoToken", err)
	}
}

func TestHostStoreLegacyFallbackFromBareTokenAndEmail(t *testing.T) {
	ring := newTestRing(t)

	// Write bare token and email keys (oldest legacy format)
	bare := &KeyringStore{ring: ring}
	if err := bare.SetToken("old-token"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	if err := bare.SetEmail("old@example.com"); err != nil {
		t.Fatalf("SetEmail: %v", err)
	}

	store := NewHostStore(ring, config.DefaultAPIHost)
	cred, err := store.GetCredential()
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if cred.Token != "old-token" || cred.Email != "old@example.com" {
		t.Fatalf("credential = %#v", cred)
	}
}

func TestHostStoreDeleteDefaultHostCleansLegacyKeys(t *testing.T) {
	ring := newTestRing(t)

	// Write a bare credential key (simulating pre-migration state)
	bare := &KeyringStore{ring: ring}
	if err := bare.SetCredential(Credential{Token: "legacy-token", Email: "legacy@example.com"}); err != nil {
		t.Fatalf("SetCredential bare: %v", err)
	}

	store := NewHostStore(ring, config.DefaultAPIHost)

	// Read triggers backfill to host-scoped key
	if _, err := store.GetCredential(); err != nil {
		t.Fatalf("GetCredential: %v", err)
	}

	// Delete should remove both host-scoped AND bare legacy keys
	if err := store.DeleteCredential(); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}

	// Credential should NOT reappear via legacy fallback
	if _, err := store.GetCredential(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("GetCredential after delete: got %v, want ErrNoToken", err)
	}
}
