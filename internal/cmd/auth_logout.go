package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/oauth"
	"github.com/nimbu/cli/internal/output"
)

// AuthLogoutCmd logs out and removes stored credentials.
type AuthLogoutCmd struct{}

// Run executes the logout command.
func (c *AuthLogoutCmd) Run(ctx context.Context) error {
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)

	stderr := output.WriterFromContext(ctx).Err
	envSet := envToken() != ""

	// Log out of the stored session, never NIMBU_TOKEN: that token belongs to
	// the environment (often CI) and stays valid.
	cred, err := resolverFromContext(ctx).Credential()
	if errors.Is(err, auth.ErrNoToken) {
		if envSet {
			return fmt.Errorf("%w: no stored login to log out of; NIMBU_TOKEN is set and is not affected by logout", auth.ErrNoToken)
		}
		return fmt.Errorf("%w: run 'nimbu auth login' first", auth.ErrNoToken)
	}
	if err != nil {
		return err
	}

	if cred.IsOAuth() {
		// Hold the session lock so a refresh in another process cannot save
		// a rotated session after it is deleted here.
		if unlock, err := oauth.LockSession(ctx, refreshLockPath(resolverFromContext(ctx).host)); err == nil {
			defer unlock()
		}
		// Best effort: the local credential goes either way, and revoking the
		// refresh token ends the whole session on the server.
		client := oauth.NewClient(flags.APIURL, &http.Client{Timeout: flags.Timeout})
		if cred.ClientID != "" {
			client.ClientID = cred.ClientID
		}
		if err := client.Revoke(ctx, cred.RefreshToken, "refresh_token"); err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: could not revoke the session on the server: %v\n", err)
		}
	} else {
		// Legacy API token: /auth/logout revokes it. Not readonly on purpose:
		// ending a session must work under --readonly.
		client := api.New(flags.APIURL, cred.Token).WithVersion(version).WithTimeout(flags.Timeout).WithDebug(flags.Debug)
		if err := client.Post(ctx, "/auth/logout", nil, nil); err != nil {
			return fmt.Errorf("logout request failed: %w", err)
		}
	}

	if err := DeleteStoredCredentials(ctx); err != nil {
		return err
	}
	if envSet {
		_, _ = fmt.Fprintln(stderr, "warning: NIMBU_TOKEN is still set; commands keep using it until you unset it")
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("logged out"))
	}

	if _, err := output.Fprintln(ctx, "Logged out"); err != nil {
		return err
	}
	return nil
}
