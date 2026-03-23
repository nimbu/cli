package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/output"
)

// AuthLogoutCmd logs out and removes stored credentials.
type AuthLogoutCmd struct{}

// Run executes the logout command.
func (c *AuthLogoutCmd) Run(ctx context.Context) error {
	client, err := GetAPIClient(ctx)
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
