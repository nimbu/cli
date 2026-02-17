package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// TokensRevokeCmd revokes an API token.
type TokensRevokeCmd struct {
	ID string `arg:"" help:"Token ID to revoke"`
}

// Run executes the revoke command.
func (c *TokensRevokeCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "revoke token"); err != nil {
		return err
	}

	if err := requireForce(flags, "token "+c.ID); err != nil {
		return err
	}

	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	path := "/tokens/" + url.PathEscape(c.ID)
	if err := client.Delete(ctx, path, nil); err != nil {
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
