// Package oauth implements the Nimbu CLI's OAuth 2.0 client: authorization
// server discovery, the loopback (PKCE) and device login flows, token
// revocation, and a persistent token source that refreshes the stored session.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/nimbu/cli/internal/auth"
)

// DefaultClientID is the first-party public client registered for the CLI.
const DefaultClientID = "nimbu-cli"

// ClientIDEnv overrides the client id, for development servers only.
const ClientIDEnv = "NIMBU_OAUTH_CLIENT_ID"

const (
	metadataPath    = "/.well-known/oauth-authorization-server"
	errInvalidGrant = "invalid_grant"
)

// ErrSessionExpired means the stored session can no longer be refreshed and
// the user has to log in again. It wraps auth.ErrNoToken so callers treat it
// as "not logged in".
var ErrSessionExpired = fmt.Errorf("session expired: %w", auth.ErrNoToken)

// Endpoints are the authorization server URLs the CLI talks to.
type Endpoints struct {
	Issuer                      string `json:"issuer"`
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	RevocationEndpoint          string `json:"revocation_endpoint"`
}

// Client speaks OAuth to the authorization server that fronts one Nimbu API.
type Client struct {
	APIURL     string
	ClientID   string
	HTTPClient *http.Client

	once      sync.Once
	endpoints Endpoints
}

// NewClient returns a client for the API at apiURL. The client id defaults to
// DefaultClientID unless ClientIDEnv is set.
func NewClient(apiURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		APIURL:     strings.TrimRight(strings.TrimSpace(apiURL), "/"),
		ClientID:   ClientID(),
		HTTPClient: httpClient,
	}
}

// ClientID returns the OAuth client id the CLI identifies as.
func ClientID() string {
	if id := strings.TrimSpace(os.Getenv(ClientIDEnv)); id != "" {
		return id
	}
	return DefaultClientID
}

// Endpoints discovers the server metadata once (RFC 8414) and falls back to
// the conventional Nimbu URLs when discovery is unavailable.
func (c *Client) Endpoints(ctx context.Context) Endpoints {
	c.once.Do(func() {
		fallback := FallbackEndpoints(c.APIURL)
		discovered, err := c.discover(ctx)
		if err != nil {
			c.endpoints = fallback
			return
		}
		c.endpoints = mergeEndpoints(discovered, fallback)
	})
	return c.endpoints
}

func (c *Client) discover(ctx context.Context) (Endpoints, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIURL+metadataPath, nil)
	if err != nil {
		return Endpoints{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Endpoints{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Endpoints{}, fmt.Errorf("discovery: HTTP %d", resp.StatusCode)
	}
	var meta Endpoints
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&meta); err != nil {
		return Endpoints{}, fmt.Errorf("discovery: %w", err)
	}
	if meta.TokenEndpoint == "" {
		return Endpoints{}, errors.New("discovery: missing token_endpoint")
	}
	return meta, nil
}

func mergeEndpoints(primary, fallback Endpoints) Endpoints {
	pick := func(a, b string) string {
		if strings.TrimSpace(a) != "" {
			return strings.TrimSpace(a)
		}
		return b
	}
	return Endpoints{
		Issuer:                      pick(primary.Issuer, fallback.Issuer),
		AuthorizationEndpoint:       pick(primary.AuthorizationEndpoint, fallback.AuthorizationEndpoint),
		TokenEndpoint:               pick(primary.TokenEndpoint, fallback.TokenEndpoint),
		DeviceAuthorizationEndpoint: pick(primary.DeviceAuthorizationEndpoint, fallback.DeviceAuthorizationEndpoint),
		RevocationEndpoint:          pick(primary.RevocationEndpoint, fallback.RevocationEndpoint),
	}
}

// FallbackEndpoints derives the Nimbu endpoints from the API URL: the token,
// device and revocation endpoints live on the API host, and the browser
// consent screen on the www host of the same domain (api.X -> www.X).
func FallbackEndpoints(apiURL string) Endpoints {
	apiURL = strings.TrimRight(strings.TrimSpace(apiURL), "/")
	authorizeBase := apiURL
	if parsed, err := url.Parse(apiURL); err == nil && parsed.Host != "" {
		if rest, ok := strings.CutPrefix(parsed.Host, "api."); ok {
			parsed.Host = "www." + rest
		}
		parsed.Path = ""
		authorizeBase = parsed.String()
	}
	return Endpoints{
		Issuer:                      apiURL,
		AuthorizationEndpoint:       authorizeBase + "/admin/oauth2/authorize",
		TokenEndpoint:               apiURL + "/oauth2/tokens",
		DeviceAuthorizationEndpoint: apiURL + "/oauth2/device_authorization",
		RevocationEndpoint:          apiURL + "/oauth2/revoke",
	}
}

func (c *Client) config(ctx context.Context, scopes []string, redirectURL string) *oauth2.Config {
	ep := c.Endpoints(ctx)
	return &oauth2.Config{
		ClientID:    c.ClientID,
		RedirectURL: redirectURL,
		Scopes:      scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:       ep.AuthorizationEndpoint,
			TokenURL:      ep.TokenEndpoint,
			DeviceAuthURL: ep.DeviceAuthorizationEndpoint,
			// Public client: client_id in the form body, never a Basic header.
			// Auto-detection would retry a failed request with the other
			// style, which replays single-use codes and refresh tokens.
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

func (c *Client) oauthContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, c.HTTPClient)
}

// Refresh redeems a refresh token. The server rotates it: the old token is
// dead once this returns, so callers must persist the result.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	expired := &oauth2.Token{RefreshToken: refreshToken, Expiry: time.Unix(1, 0)}
	return c.config(ctx, nil, "").TokenSource(c.oauthContext(ctx), expired).Token()
}

// Revoke revokes a token (RFC 7009). The server answers 200 for unknown
// tokens too, so an error only means the request itself failed.
func (c *Client) Revoke(ctx context.Context, token, hint string) error {
	endpoint := c.Endpoints(ctx).RevocationEndpoint
	form := url.Values{"token": {token}, "client_id": {c.ClientID}}
	if hint != "" {
		form.Set("token_type_hint", hint)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("revoke: HTTP %d", resp.StatusCode)
	}
	return nil
}

// IsInvalidGrant reports whether the server rejected a grant as invalid,
// expired or revoked.
func IsInvalidGrant(err error) bool {
	return errorCode(err) == errInvalidGrant
}

func errorCode(err error) string {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		return re.ErrorCode
	}
	return ""
}

// RefreshError is a refresh the token endpoint answered with anything but a
// new session or invalid_grant. The stored session is kept: the server did
// not say it is gone.
type RefreshError struct {
	Status int    // HTTP status of the token response
	Code   string // OAuth error code; empty when the server sent none
}

func (e *RefreshError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("refresh session: the server refused the refresh (%s, HTTP %d)", e.Code, e.Status)
	}
	return fmt.Sprintf("refresh session: the token endpoint answered HTTP %d", e.Status)
}

// Rejected reports whether the server refused this client or session
// (invalid_client, unauthorized_client, ...), so retrying cannot help and
// the user has to log in again. Otherwise the failure is on the server side
// and may pass.
func (e *RefreshError) Rejected() bool {
	if e.Status < 400 || e.Status >= 500 || e.Status == http.StatusTooManyRequests {
		return false
	}
	return e.Status == http.StatusUnauthorized ||
		(e.Code != "" && e.Code != "temporarily_unavailable" && e.Code != "server_error")
}

// refreshFailure turns a failed refresh (other than invalid_grant) into an
// error that says what went wrong without the raw response body, which for a
// proxy or server error is an HTML page.
func refreshFailure(err error) error {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		status := 0
		if re.Response != nil {
			status = re.Response.StatusCode
		}
		return &RefreshError{Status: status, Code: re.ErrorCode}
	}
	return fmt.Errorf("refresh session: %w", err)
}

// Credential converts a token response into a stored credential.
func Credential(tok *oauth2.Token, clientID string) auth.Credential {
	cred := auth.Credential{
		Token:        tok.AccessToken,
		AuthMethod:   auth.AuthMethodOAuth,
		RefreshToken: tok.RefreshToken,
		ClientID:     clientID,
	}
	if !tok.Expiry.IsZero() {
		cred.ExpiresAt = tok.Expiry.UTC()
	}
	if scope, ok := tok.Extra("scope").(string); ok {
		cred.Scopes = strings.Fields(scope)
	}
	return cred
}
