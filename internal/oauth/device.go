package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

var (
	// ErrDeviceDenied means the user denied the device login in the browser.
	ErrDeviceDenied = errors.New("the login request was denied in the browser")
	// ErrDeviceExpired means the user code expired before it was approved.
	ErrDeviceExpired = errors.New("the login code expired before it was approved")
)

// DeviceLogin configures the device authorization flow (RFC 8628).
type DeviceLogin struct {
	Scopes     []string
	DeviceName string
	// Prompt shows the user where to go and which code to enter. The code is
	// never embedded in a link: typing it by hand is the phishing guard.
	Prompt func(verificationURI, userCode string, expiresAt time.Time)
}

// LoginWithDevice requests a user code, shows it through opts.Prompt and
// polls the token endpoint until the user approves, denies or the code expires.
func (c *Client) LoginWithDevice(ctx context.Context, opts DeviceLogin) (*oauth2.Token, error) {
	cfg := c.config(ctx, opts.Scopes, "")
	var params []oauth2.AuthCodeOption
	if opts.DeviceName != "" {
		params = append(params, oauth2.SetAuthURLParam("device_name", opts.DeviceName))
	}

	da, err := cfg.DeviceAuth(c.oauthContext(ctx), params...)
	if err != nil {
		return nil, fmt.Errorf("start device login: %w", err)
	}
	if da.VerificationURI == "" || da.UserCode == "" {
		return nil, errors.New("start device login: server response lacks verification_uri or user_code")
	}
	if opts.Prompt != nil {
		opts.Prompt(da.VerificationURI, da.UserCode, da.Expiry)
	}

	tok, err := c.pollDevice(ctx, cfg.Endpoint.TokenURL, da)
	switch {
	case err == nil:
		return tok, nil
	case errorCode(err) == "access_denied":
		return nil, ErrDeviceDenied
	case errorCode(err) == "expired_token", errors.Is(err, ErrDeviceExpired):
		return nil, ErrDeviceExpired
	default:
		return nil, fmt.Errorf("device login: %w", err)
	}
}

const (
	grantTypeDeviceCode = "urn:ietf:params:oauth:grant-type:device_code"
	// slowDownStep is how much slow_down raises the poll interval, for this
	// and every later poll (RFC 8628 §3.5).
	slowDownStep = 5 * time.Second
	// maxPollRetries bounds how many polls in a row may fail in transit.
	maxPollRetries = 3
)

// pollDevice polls the token endpoint (RFC 8628 §3.4) until the user
// answers. It owns the loop instead of using x/oauth2's DeviceAccessToken,
// which gives up on the first transport error and, when called again,
// forgets the slow_down backoff: the server keeps its raised interval, so a
// client polling at the old one would get slow_down on every poll.
func (c *Client) pollDevice(ctx context.Context, tokenURL string, da *oauth2.DeviceAuthResponse) (*oauth2.Token, error) {
	interval := time.Duration(da.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	failures := 0
	for {
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		// Check the clock rather than the error: an HTTP client timeout on a
		// single poll also matches context.DeadlineExceeded.
		if !da.Expiry.IsZero() && !time.Now().Before(da.Expiry) {
			return nil, ErrDeviceExpired
		}

		tok, err := c.pollDeviceOnce(ctx, tokenURL, da.DeviceCode)
		var transport *url.Error
		switch {
		case err == nil:
			return tok, nil
		case errors.As(err, &transport):
			failures++
			if ctx.Err() != nil || failures > maxPollRetries {
				return nil, err
			}
			continue
		case failures > 0 && IsInvalidGrant(err):
			// The device code is single use. A dropped response may have
			// carried the approval; the server revoked that session when
			// the code came back, so the only way on is a new login.
			return nil, fmt.Errorf("%w: a dropped response may have carried the approval; run `nimbu auth login` again", err)
		case errorCode(err) == "authorization_pending":
		case errorCode(err) == "slow_down":
			interval += slowDownStep
		default:
			return nil, err
		}
		failures = 0
	}
}

// pollDeviceOnce makes one device access token request. Transport failures
// come back as *url.Error, error responses as *oauth2.RetrieveError.
func (c *Client) pollDeviceOnce(ctx context.Context, tokenURL, deviceCode string) (*oauth2.Token, error) {
	form := url.Values{"grant_type": {grantTypeDeviceCode}, "device_code": {deviceCode}, "client_id": {c.ClientID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, &url.Error{Op: "Post", URL: tokenURL, Err: err}
	}

	var payload struct {
		AccessToken      string `json:"access_token"`
		TokenType        string `json:"token_type"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int64  `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		ErrorURI         string `json:"error_uri"`
	}
	decodeErr := json.Unmarshal(body, &payload)
	if resp.StatusCode != http.StatusOK || decodeErr != nil || payload.AccessToken == "" {
		// No Body: for a proxy or server error it is an HTML page, not
		// something to show the user.
		return nil, &oauth2.RetrieveError{Response: resp, ErrorCode: payload.Error, ErrorDescription: payload.ErrorDescription, ErrorURI: payload.ErrorURI}
	}

	var extra map[string]any
	_ = json.Unmarshal(body, &extra)
	tok := &oauth2.Token{AccessToken: payload.AccessToken, TokenType: payload.TokenType, RefreshToken: payload.RefreshToken}
	if payload.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	}
	return tok.WithExtra(extra), nil
}
