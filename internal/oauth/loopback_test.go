package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// browserVisit simulates the browser: it follows the authorization URL to
// the loopback callback with the given query, and reports the status. It runs
// on its own goroutine, so it reports failures with t.Errorf.
func browserVisit(t *testing.T, authURL string, query func(state string) url.Values) int {
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Errorf("parse auth url: %v", err)
		return 0
	}
	params := parsed.Query()
	callback := params.Get("redirect_uri") + "?" + query(params.Get("state")).Encode()
	resp, err := http.Get(callback)
	if err != nil {
		t.Errorf("callback: %v", err)
		return 0
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestLoginWithBrowserExchangesCodeWithPKCE(t *testing.T) {
	fs := newFakeServer(t)
	var authParams url.Values
	fs.setToken(func(form url.Values) (int, map[string]any) {
		want := map[string]string{
			"grant_type":   "authorization_code",
			"code":         "code-1",
			"client_id":    "nimbu-cli",
			"redirect_uri": authParams.Get("redirect_uri"),
		}
		for key, value := range want {
			if form.Get(key) != value {
				t.Errorf("token form %s = %q, want %q", key, form.Get(key), value)
			}
		}
		if got := oauth2.S256ChallengeFromVerifier(form.Get("code_verifier")); got != authParams.Get("code_challenge") {
			t.Errorf("code_verifier does not match the challenge")
		}
		return http.StatusOK, tokenResponse("access-1", "refresh-1")
	})

	status := make(chan int, 1)
	tok, err := fs.client().LoginWithBrowser(context.Background(), BrowserLogin{
		DeviceName: "laptop",
		Open: func(authURL string) error {
			if !strings.HasPrefix(authURL, fs.URL+"/admin/oauth2/authorize?") {
				t.Errorf("auth url = %s", authURL)
			}
			parsed, _ := url.Parse(authURL)
			authParams = parsed.Query()
			go func() {
				status <- browserVisit(t, authURL, func(state string) url.Values {
					return url.Values{"code": {"code-1"}, "state": {state}, "iss": {fs.URL}}
				})
			}()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("LoginWithBrowser: %v", err)
	}
	if tok.AccessToken != "access-1" || tok.RefreshToken != "refresh-1" {
		t.Fatalf("token = %+v", tok)
	}

	for key, want := range map[string]string{
		"client_id":             "nimbu-cli",
		"response_type":         "code",
		"code_challenge_method": "S256",
		"device_name":           "laptop",
	} {
		if got := authParams.Get(key); got != want {
			t.Errorf("authorize %s = %q, want %q", key, got, want)
		}
	}
	if authParams.Has("scope") {
		t.Errorf("scope sent without --scopes: %q", authParams.Get("scope"))
	}
	if !strings.HasPrefix(authParams.Get("redirect_uri"), "http://127.0.0.1:") || !strings.HasSuffix(authParams.Get("redirect_uri"), "/callback") {
		t.Errorf("redirect_uri = %q", authParams.Get("redirect_uri"))
	}
	if authParams.Get("state") == "" || authParams.Get("code_challenge") == "" {
		t.Errorf("missing state or code_challenge: %v", authParams)
	}
	if got := <-status; got != http.StatusOK {
		t.Errorf("callback status = %d, want 200", got)
	}
}

func TestLoginWithBrowserIgnoresWrongStateAndSendsScopes(t *testing.T) {
	fs := newFakeServer(t)
	fs.setToken(func(url.Values) (int, map[string]any) {
		return http.StatusOK, tokenResponse("access-1", "refresh-1")
	})

	_, err := fs.client().LoginWithBrowser(context.Background(), BrowserLogin{
		Scopes: []string{"read_channels", "write_channels"},
		Open: func(authURL string) error {
			parsed, _ := url.Parse(authURL)
			if got := parsed.Query().Get("scope"); got != "read_channels write_channels" {
				t.Errorf("scope = %q", got)
			}
			go func() {
				forged := browserVisit(t, authURL, func(string) url.Values {
					return url.Values{"code": {"evil"}, "state": {"forged"}}
				})
				if forged != http.StatusBadRequest {
					t.Errorf("wrong state status = %d, want 400", forged)
				}
				browserVisit(t, authURL, func(state string) url.Values {
					return url.Values{"code": {"code-1"}, "state": {state}}
				})
			}()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("LoginWithBrowser: %v", err)
	}
}

func TestLoginWithBrowserRejectsBadCallbacks(t *testing.T) {
	cases := map[string]struct {
		query   func(state, issuer string) url.Values
		wantErr string
	}{
		"error param": {
			query: func(state, _ string) url.Values {
				return url.Values{"error": {"access_denied"}, "error_description": {"User denied"}, "state": {state}}
			},
			wantErr: "access_denied (User denied)",
		},
		"issuer mismatch": {
			query: func(state, _ string) url.Values {
				return url.Values{"code": {"c"}, "state": {state}, "iss": {"https://evil.example"}}
			},
			wantErr: "unexpected issuer",
		},
		"missing code": {
			query:   func(state, issuer string) url.Values { return url.Values{"state": {state}, "iss": {issuer}} },
			wantErr: "no code",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fs := newFakeServer(t)
			_, err := fs.client().LoginWithBrowser(context.Background(), BrowserLogin{
				Open: func(authURL string) error {
					go browserVisit(t, authURL, func(state string) url.Values { return tc.query(state, fs.URL) })
					return nil
				},
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoginWithBrowserTimesOut(t *testing.T) {
	fs := newFakeServer(t)
	var announced string
	_, err := fs.client().LoginWithBrowser(context.Background(), BrowserLogin{
		Timeout:  50 * time.Millisecond,
		Announce: func(authURL string) { announced = authURL },
		Open:     func(string) error { return errors.New("no browser") },
	})
	if !errors.Is(err, ErrLoginTimeout) {
		t.Fatalf("error = %v, want ErrLoginTimeout", err)
	}
	if announced == "" {
		t.Fatal("authorization URL was not announced")
	}
}
