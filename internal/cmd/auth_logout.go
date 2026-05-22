package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/output"
)

// AuthLogoutCmd logs out and removes stored credentials.
type AuthLogoutCmd struct{}

// Run executes the logout command.
func (c *AuthLogoutCmd) Run(ctx context.Context) error {
	client, err := newAuthSessionAPIClient(ctx)
	if err != nil {
		return err
	}

	if err := client.Post(ctx, "/auth/logout", nil, nil); err != nil {
		return fmt.Errorf("logout request failed: %w", err)
	}

	if err := DeleteStoredCredentials(ctx); err != nil {
		return err
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

func newAuthSessionAPIClient(ctx context.Context) (*api.Client, error) {
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)

	token, err := ResolveAuthToken(ctx)
	if err != nil {
		if errors.Is(err, auth.ErrNoToken) {
			return nil, fmt.Errorf("%w: run 'nimbu auth login' first", auth.ErrNoToken)
		}
		return nil, err
	}

	client := api.New(flags.APIURL, token)
	client = client.WithVersion(version)
	client = client.WithTimeout(flags.Timeout)
	client = client.WithDebug(flags.Debug)
	return client, nil
}
