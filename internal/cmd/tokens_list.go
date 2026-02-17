package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// TokensListCmd lists API tokens.
type TokensListCmd struct{}

// Run executes the list command.
func (c *TokensListCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	tokens, err := api.List[api.Token](ctx, client, "/tokens")
	if err != nil {
		return fmt.Errorf("list tokens: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, tokens)
	}

	if mode.Plain {
		for _, t := range tokens {
			if err := output.Plain(ctx, t.ID, t.Name); err != nil {
				return err
			}
		}
		return nil
	}

	fields := []string{"id", "name"}
	headers := []string{"ID", "NAME"}
	return output.WriteTable(ctx, tokens, fields, headers)
}
