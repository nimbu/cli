package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/nimbu/cli/internal/auth"
)

func TestEndpointsUsesDiscoveredMetadata(t *testing.T) {
	fs := newFakeServer(t)
	got := fs.client().Endpoints(context.Background())
	if got.TokenEndpoint != fs.URL+"/oauth2/tokens" || got.RevocationEndpoint != fs.URL+"/oauth2/revoke" {
		t.Fatalf("endpoints = %+v", got)
	}
	if got.Issuer != fs.URL {
		t.Fatalf("issuer = %q, want %q", got.Issuer, fs.URL)
	}
}

func TestEndpointsFallsBackWhenDiscoveryFails(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	got := NewClient(srv.URL, srv.Client()).Endpoints(context.Background())
	if want := FallbackEndpoints(srv.URL); got != want {
		t.Fatalf("endpoints = %+v, want %+v", got, want)
	}
}

func TestFallbackEndpoints(t *testing.T) {
	cases := map[string]Endpoints{
		"https://api.nimbu.io/": {
			Issuer:                      "https://api.nimbu.io",
			AuthorizationEndpoint:       "https://www.nimbu.io/admin/oauth2/authorize",
			TokenEndpoint:               "https://api.nimbu.io/oauth2/tokens",
			DeviceAuthorizationEndpoint: "https://api.nimbu.io/oauth2/device_authorization",
			RevocationEndpoint:          "https://api.nimbu.io/oauth2/revoke",
		},
		"http://localhost:3000": {
			Issuer:                      "http://localhost:3000",
			AuthorizationEndpoint:       "http://localhost:3000/admin/oauth2/authorize",
			TokenEndpoint:               "http://localhost:3000/oauth2/tokens",
			DeviceAuthorizationEndpoint: "http://localhost:3000/oauth2/device_authorization",
			RevocationEndpoint:          "http://localhost:3000/oauth2/revoke",
		},
	}
	for apiURL, want := range cases {
		if got := FallbackEndpoints(apiURL); got != want {
			t.Errorf("FallbackEndpoints(%q) = %+v, want %+v", apiURL, got, want)
		}
	}
}

func TestClientIDOverride(t *testing.T) {
	if got := ClientID(); got != DefaultClientID {
		t.Fatalf("ClientID() = %q, want %q", got, DefaultClientID)
	}
	t.Setenv(ClientIDEnv, "dev-cli")
	if got := ClientID(); got != "dev-cli" {
		t.Fatalf("ClientID() = %q, want dev-cli", got)
	}
}

func TestRevokeSendsRefreshTokenAsPublicClient(t *testing.T) {
	fs := newFakeServer(t)
	if err := fs.client().Revoke(context.Background(), "refresh-1", "refresh_token"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	want := url.Values{"token": {"refresh-1"}, "token_type_hint": {"refresh_token"}, "client_id": {"nimbu-cli"}}
	if len(fs.revoked) != 1 || !reflect.DeepEqual(fs.revoked[0], want) {
		t.Fatalf("revoke requests = %v, want [%v]", fs.revoked, want)
	}
}

func TestRefreshRotatesAndDetectsInvalidGrant(t *testing.T) {
	fs := newFakeServer(t)
	fs.setToken(func(form url.Values) (int, map[string]any) {
		if form.Get("grant_type") != "refresh_token" || form.Get("client_id") != "nimbu-cli" {
			t.Errorf("refresh form = %v", form)
		}
		if form.Get("refresh_token") == "dead" {
			return http.StatusBadRequest, map[string]any{"error": "invalid_grant"}
		}
		return http.StatusOK, tokenResponse("access-2", "refresh-2")
	})

	tok, err := fs.client().Refresh(context.Background(), "refresh-1")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if tok.AccessToken != "access-2" || tok.RefreshToken != "refresh-2" {
		t.Fatalf("token = %+v", tok)
	}

	_, err = fs.client().Refresh(context.Background(), "dead")
	if !IsInvalidGrant(err) {
		t.Fatalf("Refresh(dead) error = %v, want invalid_grant", err)
	}
}

func TestCredentialFromToken(t *testing.T) {
	expiry := time.Date(2026, 9, 25, 12, 30, 0, 0, time.UTC)
	tok := (&oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: expiry}).
		WithExtra(map[string]any{"scope": "read_channels write_pages"})

	got := Credential(tok, "nimbu-cli")
	want := auth.Credential{
		Token:        "a",
		AuthMethod:   auth.AuthMethodOAuth,
		RefreshToken: "r",
		ExpiresAt:    expiry,
		Scopes:       []string{"read_channels", "write_pages"},
		ClientID:     "nimbu-cli",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Credential = %+v, want %+v", got, want)
	}
}
