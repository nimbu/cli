package auth

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLegacyCredentialJSONDecodesAsStaticToken(t *testing.T) {
	cred, err := decodeCredential([]byte(`{"token":"tok","email":"Me@Example.com","created_at":"2025-01-02T03:04:05Z"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cred.IsOAuth() || cred.Token != "tok" || cred.Email != "me@example.com" || !cred.ExpiresAt.IsZero() || cred.RefreshToken != "" {
		t.Fatalf("legacy credential = %+v", cred)
	}
}

func TestStaticCredentialJSONOmitsOAuthFields(t *testing.T) {
	data, err := json.Marshal(Credential{Token: "tok"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"auth_method", "refresh_token", "expires_at", "scopes", "client_id"} {
		if strings.Contains(string(data), key) {
			t.Fatalf("static credential JSON %s contains %q", data, key)
		}
	}
}

func TestOAuthCredentialRoundTripsThroughHostStore(t *testing.T) {
	store := NewHostStore(newTestRing(t), "api.example.test")
	want := Credential{
		Token:        "access",
		Email:        "me@example.com",
		CreatedAt:    time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC),
		AuthMethod:   AuthMethodOAuth,
		RefreshToken: "refresh",
		ExpiresAt:    time.Date(2026, 9, 25, 10, 30, 0, 0, time.UTC),
		Scopes:       []string{"read_channels", "write_channels"},
		ClientID:     "nimbu-cli",
	}
	if err := store.SetCredential(want); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	got, err := store.GetCredential()
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

// An older CLI decodes the OAuth credential with its own, smaller struct and
// keeps working with the access token until it expires.
func TestOAuthCredentialJSONReadableByOlderCLI(t *testing.T) {
	data, err := json.Marshal(Credential{Token: "access", Email: "me@example.com", AuthMethod: AuthMethodOAuth, RefreshToken: "refresh"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var legacy struct {
		Token     string    `json:"token"`
		Email     string    `json:"email,omitempty"`
		CreatedAt time.Time `json:"created_at,omitempty"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil || legacy.Token != "access" || legacy.Email != "me@example.com" {
		t.Fatalf("older CLI decode = %+v, %v", legacy, err)
	}
}
