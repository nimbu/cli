package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// TokensGetCmd fetches token details.
type TokensGetCmd struct {
	ID string `arg:"" help:"Token ID"`
}

// Run executes the get command.
func (c *TokensGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var token api.Token
	path := "/tokens/" + url.PathEscape(c.ID)
	if err := client.Get(ctx, path, &token); err != nil {
		return fmt.Errorf("get token: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, token)
	}

	if mode.Plain {
		return output.Plain(ctx, token.ID, token.Name)
	}

	fmt.Printf("ID:         %s\n", token.ID)
	fmt.Printf("Name:       %s\n", token.Name)
	fmt.Printf("Expires at:  %s\n", token.ExpiresAt)

	return nil
}
