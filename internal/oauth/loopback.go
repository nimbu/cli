package oauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// DefaultBrowserTimeout bounds how long the loopback flow waits for the user.
const DefaultBrowserTimeout = 5 * time.Minute

// ErrLoginTimeout means the user did not finish the browser login in time.
var ErrLoginTimeout = errors.New("timed out waiting for the browser login to finish")

// BrowserLogin configures the loopback authorization code flow (RFC 8252).
type BrowserLogin struct {
	Scopes     []string
	DeviceName string
	Timeout    time.Duration
	// Announce receives the authorization URL before the browser opens, so
	// the user can open it by hand when no browser starts.
	Announce func(authURL string)
	// Open launches the browser. Failures are ignored: the URL was announced.
	Open func(authURL string) error
}

type callbackResult struct {
	code string
	err  error
}

// LoginWithBrowser runs the authorization code flow with PKCE (S256) against
// a one-shot callback server on 127.0.0.1 and exchanges the code for tokens.
func (c *Client) LoginWithBrowser(ctx context.Context, opts BrowserLogin) (*oauth2.Token, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start callback server: %w", err)
	}
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)

	state := rand.Text()
	verifier := oauth2.GenerateVerifier()
	cfg := c.config(ctx, opts.Scopes, redirectURL)
	params := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if opts.DeviceName != "" {
		params = append(params, oauth2.SetAuthURLParam("device_name", opts.DeviceName))
	}
	authURL := cfg.AuthCodeURL(state, params...)

	results := make(chan callbackResult, 1)
	server := &http.Server{
		Handler:           callbackHandler(state, c.Endpoints(ctx).Issuer, results),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if opts.Announce != nil {
		opts.Announce(authURL)
	}
	if opts.Open != nil {
		_ = opts.Open(authURL)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultBrowserTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var result callbackResult
	select {
	case result = <-results:
	case <-timer.C:
		return nil, ErrLoginTimeout
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if result.err != nil {
		return nil, result.err
	}

	tok, err := cfg.Exchange(c.oauthContext(ctx), result.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchange authorization code: %w", err)
	}
	return tok, nil
}

// callbackHandler accepts exactly one callback carrying the expected state.
// Requests with a missing or wrong state (stray tabs, other local software)
// get an error page but do not end the flow.
func callbackHandler(state, issuer string, results chan<- callbackResult) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1 {
			writeCallbackPage(w, http.StatusBadRequest, "This login link is invalid or has expired. Run `nimbu auth login` again.")
			return
		}
		result := parseCallback(query, issuer)
		select {
		case results <- result:
		default:
			writeCallbackPage(w, http.StatusConflict, "This login was already handled. You can close this tab.")
			return
		}
		if result.err != nil {
			writeCallbackPage(w, http.StatusBadRequest, "Login failed: "+result.err.Error()+". Return to your terminal.")
			return
		}
		writeCallbackPage(w, http.StatusOK, "You are logged in to the Nimbu CLI. You can close this tab and return to your terminal.")
	})
	return mux
}

func parseCallback(query url.Values, issuer string) callbackResult {
	if iss := query.Get("iss"); iss != "" && !sameIssuer(iss, issuer) {
		// RFC 9207: a response from another issuer is a mix-up attempt.
		return callbackResult{err: fmt.Errorf("unexpected issuer %q", iss)}
	}
	if code := query.Get("error"); code != "" {
		msg := "authorization failed: " + code
		if desc := query.Get("error_description"); desc != "" {
			msg += " (" + desc + ")"
		}
		return callbackResult{err: errors.New(msg)}
	}
	code := query.Get("code")
	if code == "" {
		return callbackResult{err: errors.New("authorization response has no code")}
	}
	return callbackResult{code: code}
}

func sameIssuer(a, b string) bool {
	return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

func writeCallbackPage(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Nimbu CLI</title>`+
		`<meta name="viewport" content="width=device-width,initial-scale=1"></head>`+
		`<body style="font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;line-height:1.5">`+
		`<h1 style="font-size:1.25rem">Nimbu CLI</h1><p>%s</p></body></html>`, html.EscapeString(message))
}
