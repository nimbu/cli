package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// TokensCreateCmd creates a new API token.
type TokensCreateCmd struct {
	Name   string   `help:"Token name" required:""`
	Scopes []string `help:"Token scopes (comma-separated)"`
}

// Run executes the create command.
func (c *TokensCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("cannot create token in readonly mode")
	}

	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	data := map[string]any{
		"name": c.Name,
	}
	if len(c.Scopes) > 0 {
		data["scopes"] = c.Scopes
	}

	var token api.Token
	if err := client.Post(ctx, "/tokens", data, &token); err != nil {
		return fmt.Errorf("create token: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, token)
	}

	if mode.Plain {
		// Plain mode outputs just the token for easy piping
		return output.Plain(ctx, token.Token)
	}

	fmt.Printf("Token created: %s\n", token.ID)
	fmt.Printf("Token value: %s\n", token.Token)
	fmt.Println("\nSave this token - it won't be shown again!")

	return nil
}
