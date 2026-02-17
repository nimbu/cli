package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/output"
)

// AuthLogoutCmd logs out and removes stored credentials.
type AuthLogoutCmd struct{}

// Run executes the logout command.
func (c *AuthLogoutCmd) Run(ctx context.Context) error {
	store, err := auth.OpenDefault()
	if err != nil {
		return fmt.Errorf("open keyring: %w", err)
	}

	ks, ok := store.(*auth.KeyringStore)
	if !ok {
		return fmt.Errorf("unexpected store type")
	}

	if err := ks.DeleteCredential(); err != nil {
		return fmt.Errorf("delete credentials: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("logged out"))
	}

	fmt.Println("Logged out")
	return nil
}
