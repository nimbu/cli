package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/output"
)

// AuthStatusCmd shows authentication status.
type AuthStatusCmd struct{}

// Run executes the status command.
func (c *AuthStatusCmd) Run(ctx context.Context) error {
	cred, err := ResolveAuthCredential(ctx)
	hasToken := true
	if errors.Is(err, auth.ErrNoToken) {
		hasToken = false
	} else if err != nil {
		return err
	}
	email := cred.Email

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]any{
			"logged_in": hasToken,
			"email":     email,
		})
	}

	if mode.Plain {
		if hasToken {
			return output.Plain(ctx, "logged_in", email)
		}
		return output.Plain(ctx, "logged_out", "")
	}

	if hasToken {
		if email != "" {
			fmt.Printf("Logged in as %s\n", email)
		} else {
			fmt.Println("Logged in")
		}
	} else {
		fmt.Println("Not logged in")
	}

	return nil
}
