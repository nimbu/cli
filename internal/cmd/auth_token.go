package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/output"
)

// AuthTokenCmd prints the access token.
type AuthTokenCmd struct{}

// Run executes the token command.
func (c *AuthTokenCmd) Run(ctx context.Context) error {
	store, err := auth.OpenDefault()
	if err != nil {
		return fmt.Errorf("open keyring: %w", err)
	}

	token, err := store.GetToken()
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]string{"token": token})
	}

	// Plain and human output are the same - just the token
	fmt.Println(token)
	return nil
}
