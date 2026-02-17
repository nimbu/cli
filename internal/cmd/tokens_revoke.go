package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/output"
)

// TokensRevokeCmd revokes an API token.
type TokensRevokeCmd struct {
	ID string `arg:"" help:"Token ID to revoke"`
}

// Run executes the revoke command.
func (c *TokensRevokeCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("cannot revoke token in readonly mode")
	}

	if !flags.Force {
		return fmt.Errorf("use --force to confirm token revocation")
	}

	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	if err := client.Delete(ctx, "/tokens/"+c.ID, nil); err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("revoked"))
	}

	if mode.Plain {
		return output.Plain(ctx, c.ID, "revoked")
	}

	fmt.Printf("Revoked token %s\n", c.ID)
	return nil
}
