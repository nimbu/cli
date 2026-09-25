package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/cli/browser"
	"golang.org/x/oauth2"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/oauth"
	"github.com/nimbu/cli/internal/output"
)

// openBrowser launches the system browser; swapped in tests.
var openBrowser = func(url string) error {
	browser.Stdout, browser.Stderr = io.Discard, io.Discard
	return browser.OpenURL(url)
}

// runOAuthLogin logs in through the browser (loopback + PKCE) or, with
// --device or on a headless machine, with a one-time code.
func (c *AuthLoginCmd) runOAuthLogin(ctx context.Context, flags *RootFlags, host string) error {
	stderr := output.WriterFromContext(ctx).Err
	client := oauth.NewClient(flags.APIURL, &http.Client{Timeout: flags.Timeout})
	scopes := splitScopes(c.Scopes)
	deviceName := loginDeviceName()

	useDevice := c.Device
	if !useDevice && isHeadless(os.Getenv, runtime.GOOS) {
		_, _ = fmt.Fprintln(stderr, "No local browser available (SSH session or no display): logging in with a one-time code.")
		useDevice = true
	}

	var tok *oauth2.Token
	var err error
	if useDevice {
		tok, err = client.LoginWithDevice(ctx, oauth.DeviceLogin{
			Scopes:     scopes,
			DeviceName: deviceName,
			Prompt: func(verificationURI, userCode string, expiresAt time.Time) {
				printDevicePrompt(stderr, verificationURI, userCode, expiresAt)
			},
		})
	} else {
		tok, err = client.LoginWithBrowser(ctx, oauth.BrowserLogin{
			Scopes:     scopes,
			DeviceName: deviceName,
			Announce: func(authURL string) {
				_, _ = fmt.Fprintf(stderr, "Opening your browser to log in. If it does not open, visit:\n\n  %s\n\nWaiting for you to finish in the browser...\n", authURL)
			},
			Open: openBrowser,
		})
	}
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	cred := oauth.Credential(tok, client.ClientID)
	var user api.User
	userClient := api.New(flags.APIURL, cred.Token).WithVersion(version).WithTimeout(flags.Timeout).WithDebug(flags.Debug)
	if err := userClient.Get(ctx, "/user", &user); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: logged in, but could not load your profile: %v\n", err)
	}
	cred.Email = user.Email

	if err := storeCredential(ctx, host, cred); err != nil {
		return err
	}

	if output.FromContext(ctx).JSON {
		payload := map[string]any{
			"status":      "logged_in",
			"email":       cred.Email,
			"host":        host,
			"auth_method": cred.AuthMethod,
		}
		if !cred.ExpiresAt.IsZero() {
			payload["expires_at"] = cred.ExpiresAt
		}
		return output.JSON(ctx, payload)
	}

	who := cred.Email
	if user.Name != "" {
		who = fmt.Sprintf("%s (%s)", user.Name, cred.Email)
	}
	if who == "" {
		_, err = output.Fprintf(ctx, "Logged in to %s\n", host)
	} else {
		_, err = output.Fprintf(ctx, "Logged in to %s as %s\n", host, who)
	}
	return err
}

func printDevicePrompt(w io.Writer, verificationURI, userCode string, expiresAt time.Time) {
	expiry := ""
	if !expiresAt.IsZero() {
		expiry = fmt.Sprintf(" The code expires in %d minutes.", max(1, int(time.Until(expiresAt).Round(time.Minute).Minutes())))
	}
	_, _ = fmt.Fprintf(w, "To log in, open this page in a browser on any device:\n\n  %s\n\nand enter this code:\n\n  %s\n\n"+
		"Only enter the code on a page you opened yourself. Nimbu never asks you for it.%s\nWaiting for approval...\n",
		verificationURI, userCode, expiry)
}

// isHeadless reports whether no local browser can reach the loopback
// callback: an SSH session, or Linux without a display server.
func isHeadless(getenv func(string) string, goos string) bool {
	if getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" {
		return true
	}
	return goos == "linux" && getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == ""
}

// splitScopes parses a comma- or space-separated scope list.
func splitScopes(raw string) []string {
	var scopes []string
	seen := map[string]bool{}
	for _, scope := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' }) {
		if !seen[scope] {
			seen[scope] = true
			scopes = append(scopes, scope)
		}
	}
	return scopes
}

// loginDeviceName names this machine on the consent screen, the sessions
// list and the login e-mail.
func loginDeviceName() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	name = strings.TrimSpace(name)
	if len(name) > 100 {
		name = name[:100]
	}
	return name
}
