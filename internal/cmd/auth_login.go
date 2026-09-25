package cmd

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/oauth"
	"github.com/nimbu/cli/internal/output"
)

// AuthLoginCmd logs in to Nimbu.
type AuthLoginCmd struct {
	Device bool   `help:"Log in with a one-time code entered in a browser on any device (SSH, containers)"`
	Scopes string `help:"Comma-separated OAuth scopes to request (default: full CLI access)"`
	// Deliberately not bound to NIMBU_TOKEN: an exported CI token must not
	// turn a login into storing that token.
	Token string `help:"Store an existing API token instead of logging in" short:"t"`

	// Deprecated email/password login, kept for scripts. Either flag selects it.
	Email     string `help:"Deprecated: log in with email and password" short:"e" hidden:""`
	Password  string `help:"Deprecated: password for --email login" short:"p" hidden:""`
	ExpiresIn int    `help:"Deprecated: token lifetime in seconds for --email login" short:"x" default:"7776000" hidden:""`
}

// Run executes the login command.
func (c *AuthLoginCmd) Run(ctx context.Context, flags *RootFlags) error {
	host := apps.NormalizeHost(flags.APIURL)
	if envToken() != "" {
		_, _ = fmt.Fprintln(output.WriterFromContext(ctx).Err,
			"warning: NIMBU_TOKEN is set and overrides this login; unset it to use the new session")
	}

	switch {
	case c.Token != "":
		return c.storeToken(ctx, c.Token, "", host)
	case c.Email != "" || c.Password != "":
		_, _ = fmt.Fprintln(output.WriterFromContext(ctx).Err,
			"warning: password login is deprecated and will be removed; run `nimbu auth login` to log in with your browser")
		return c.runPasswordLogin(ctx, flags, host)
	default:
		return c.runOAuthLogin(ctx, flags, host)
	}
}

func (c *AuthLoginCmd) runPasswordLogin(ctx context.Context, flags *RootFlags, host string) error {
	email := c.Email
	if email == "" {
		if flags.NoInput {
			return fmt.Errorf("--email required with --no-input")
		}
		var err error
		email, err = prompt("Email: ")
		if err != nil {
			return fmt.Errorf("read email: %w", err)
		}
	}

	password := c.Password
	if password == "" {
		if flags.NoInput {
			return fmt.Errorf("--password required with --no-input")
		}
		var err error
		password, err = promptPassword("Password: ")
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
	}

	client := api.New(flags.APIURL, "").WithVersion(version)

	resp, err := loginWithCredentials(ctx, client, email, password, c.ExpiresIn, flags.NoInput, prompt)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	if err := c.storeToken(ctx, resp.Token, email, host); err != nil {
		return err
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]any{
			"status": "logged_in",
			"email":  email,
			"user":   resp.User,
		})
	}

	if _, err := output.Fprintf(ctx, "Logged in as %s\n", email); err != nil {
		return err
	}
	return nil
}

func loginWithCredentials(ctx context.Context, client *api.Client, email, password string, expiresIn int, noInput bool, promptTwoFactor func(string) (string, error)) (api.AuthResponse, error) {
	if expiresIn <= 0 {
		expiresIn = 60 * 60 * 24 * 90 // the server clamps password logins to 90 days
	}

	resp, err := performLogin(ctx, client, email, password, expiresIn, "")
	if err == nil {
		return resp, nil
	}

	if !isTwoFactorRequired(err) {
		return api.AuthResponse{}, err
	}

	if noInput {
		return api.AuthResponse{}, fmt.Errorf("two-factor code required with --no-input")
	}

	secondFactor, err := promptTwoFactor("Two-factor code: ")
	if err != nil {
		return api.AuthResponse{}, fmt.Errorf("read two-factor code: %w", err)
	}

	return performLogin(ctx, client, email, password, expiresIn, secondFactor)
}

func performLogin(ctx context.Context, client *api.Client, email, password string, expiresIn int, secondFactor string) (api.AuthResponse, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "nimbu"
	}

	var resp api.AuthResponse
	opts := []api.RequestOption{api.WithHeader("Authorization", "Basic "+basicAuthHeader(email, password))}
	if secondFactor != "" {
		opts = append(opts, api.WithHeader("X-Nimbu-Two-Factor", secondFactor))
	}

	err = client.Post(ctx, "/auth/login", api.LoginRequest{
		Description: "Nimbu login from " + hostname,
		ExpiresIn:   expiresIn,
	}, &resp, opts...)
	return resp, err
}

func isTwoFactorRequired(err error) bool {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		return strings.TrimSpace(apiErr.Code) == "210"
	}
	return false
}

func basicAuthHeader(email, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(email + ":" + password))
}

func (c *AuthLoginCmd) storeToken(ctx context.Context, token, email, host string) error {
	return storeCredential(ctx, host, auth.Credential{Token: token, Email: email})
}

// storeCredential replaces the credential stored for host, drops the
// session cache so the rest of this process uses the new login, and ends the
// OAuth session it replaced on the server instead of leaving it active until
// it expires. Every login path goes through here.
func storeCredential(ctx context.Context, host string, cred auth.Credential) error {
	previous, err := replaceStoredCredential(ctx, host, cred)
	if err != nil {
		return err
	}
	if previous.RefreshToken != "" && previous.RefreshToken != cred.RefreshToken {
		revokeReplacedSession(ctx, host, previous)
	}
	return nil
}

// replaceStoredCredential stores cred for host and returns the OAuth session
// it replaced, if any. It holds the session lock, so a refresh running in
// another process finishes first and the session read here is the latest
// one, and that refresh cannot write back over the new login.
func replaceStoredCredential(ctx context.Context, host string, cred auth.Credential) (auth.Credential, error) {
	if unlock, err := oauth.LockSession(ctx, refreshLockPath(host)); err == nil {
		defer unlock()
	}
	// Without the lock (another process stuck refreshing) the login still
	// goes through: a refresh re-reads the store before it writes and leaves
	// a session it did not start from alone.

	store, err := openAuthStore(host)
	if err != nil {
		return auth.Credential{}, fmt.Errorf("open keyring: %w", err)
	}
	var previous auth.Credential
	if stored, err := store.GetCredential(); err == nil && stored.IsOAuth() {
		previous = stored
	}
	if err := store.SetCredential(cred); err != nil {
		return auth.Credential{}, fmt.Errorf("store credential: %w", err)
	}

	resolverForHost(ctx, host).forget()
	return previous, nil
}

// revokeReplacedSession revokes a replaced session's refresh token, which
// ends the whole session on the server. Best effort: the new login stands
// either way, and the session also shows under CLI sessions in the admin.
func revokeReplacedSession(ctx context.Context, host string, previous auth.Credential) {
	flags, _ := ctx.Value(rootFlagsKey{}).(*RootFlags)
	if flags == nil || apps.NormalizeHost(flags.APIURL) != host {
		return
	}
	client := oauth.NewClient(flags.APIURL, &http.Client{Timeout: flags.Timeout})
	if previous.ClientID != "" {
		client.ClientID = previous.ClientID
	}
	_ = client.Revoke(ctx, previous.RefreshToken, "refresh_token")
}

func prompt(message string) (string, error) {
	fmt.Fprint(os.Stderr, message)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptPassword(message string) (string, error) {
	fmt.Fprint(os.Stderr, message)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr) // newline after password
	if err != nil {
		return "", err
	}
	return string(password), nil
}
