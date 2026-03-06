package auth

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/99designs/keyring"
)

type fakeKeyring struct {
	mu     sync.Mutex
	items  map[string]keyring.Item
	setErr map[string]error
}

func newFakeKeyring() *fakeKeyring {
	return &fakeKeyring{
		items:  map[string]keyring.Item{},
		setErr: map[string]error{},
	}
}

func (r *fakeKeyring) Get(key string) (keyring.Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[key]
	if !ok {
		return keyring.Item{}, keyring.ErrKeyNotFound
	}
	item.Data = append([]byte(nil), item.Data...)
	return item, nil
}

func (r *fakeKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	item, err := r.Get(key)
	if err != nil {
		return keyring.Metadata{}, err
	}
	item.Data = nil
	return keyring.Metadata{Item: &item}, nil
}

func (r *fakeKeyring) Set(item keyring.Item) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.setErr[item.Key]; err != nil {
		return err
	}
	item.Data = append([]byte(nil), item.Data...)
	r.items[item.Key] = item
	return nil
}

func (r *fakeKeyring) Remove(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, key)
	return nil
}

func (r *fakeKeyring) Keys() ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.items))
	for key := range r.items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func TestKeyringStoreGetCredentialReadsCombined(t *testing.T) {
	store := &KeyringStore{ring: newFakeKeyring()}
	if err := store.SetCredential(Credential{Token: "tok", Email: "Me@Example.com"}); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}

	cred, err := store.GetCredential()
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if cred.Token != "tok" || cred.Email != "me@example.com" {
		t.Fatalf("credential = %#v", cred)
	}
}

func TestKeyringStoreGetCredentialFallsBackLegacyAndBackfills(t *testing.T) {
	ring := newFakeKeyring()
	store := &KeyringStore{ring: ring}
	if err := store.SetToken("legacy-token"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	if err := store.SetEmail("Me@Example.com"); err != nil {
		t.Fatalf("SetEmail: %v", err)
	}

	cred, err := store.GetCredential()
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if cred.Token != "legacy-token" || cred.Email != "me@example.com" {
		t.Fatalf("credential = %#v", cred)
	}

	item, err := ring.Get(credentialKey)
	if err != nil {
		t.Fatalf("combined credential not backfilled: %v", err)
	}
	stored, err := decodeCredential(item.Data)
	if err != nil {
		t.Fatalf("decodeCredential: %v", err)
	}
	if stored.Token != "legacy-token" || stored.Email != "me@example.com" {
		t.Fatalf("stored credential = %#v", stored)
	}
}

func TestKeyringStoreGetCredentialFallsBackLegacyWithoutEmail(t *testing.T) {
	store := &KeyringStore{ring: newFakeKeyring()}
	if err := store.SetToken("legacy-token"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	cred, err := store.GetCredential()
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if cred.Token != "legacy-token" || cred.Email != "" {
		t.Fatalf("credential = %#v", cred)
	}
}

func TestKeyringStoreGetCredentialReturnsErrNoTokenWhenMissing(t *testing.T) {
	store := &KeyringStore{ring: newFakeKeyring()}
	if _, err := store.GetCredential(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("GetCredential err = %v, want ErrNoToken", err)
	}
}

func TestKeyringStoreGetCredentialPreservesDecodeError(t *testing.T) {
	ring := newFakeKeyring()
	if err := ring.Set(keyring.Item{Key: credentialKey, Data: []byte("{not-json")}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	store := &KeyringStore{ring: ring}

	if _, err := store.GetCredential(); err == nil || !strings.Contains(err.Error(), "decode credential:") {
		t.Fatalf("GetCredential err = %v", err)
	}
}

func TestKeyringStoreGetCredentialIgnoresBackfillErrors(t *testing.T) {
	ring := newFakeKeyring()
	ring.setErr[credentialKey] = errors.New("boom")
	store := &KeyringStore{ring: ring}
	if err := store.SetToken("legacy-token"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	cred, err := store.GetCredential()
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if cred.Token != "legacy-token" || cred.Email != "" {
		t.Fatalf("credential = %#v", cred)
	}
}

func TestKeyringStoreDeleteCredentialRemovesCombinedAndLegacyKeys(t *testing.T) {
	ring := newFakeKeyring()
	store := &KeyringStore{ring: ring}
	if err := store.SetCredential(Credential{Token: "tok", Email: "me@example.com"}); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	if err := store.SetToken("legacy-token"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	if err := store.SetEmail("me@example.com"); err != nil {
		t.Fatalf("SetEmail: %v", err)
	}

	if err := store.DeleteCredential(); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}

	keys, err := ring.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("keys = %v, want empty", keys)
	}
}

func TestDecodeCredentialNormalizesStoredValues(t *testing.T) {
	data, err := json.Marshal(Credential{Token: " tok ", Email: "Me@Example.com"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	cred, err := decodeCredential(data)
	if err != nil {
		t.Fatalf("decodeCredential: %v", err)
	}
	if cred.Token != "tok" || cred.Email != "me@example.com" {
		t.Fatalf("credential = %#v", cred)
	}
}
