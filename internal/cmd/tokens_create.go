package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// TokensCreateCmd creates a new API token.
type TokensCreateCmd struct {
	Name        string   `help:"Token name"`
	Scopes      []string `help:"Token scopes (comma-separated)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=deploy, scopes:=[\"read\"])"`
}

// Run executes the create command.
func (c *TokensCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create token"); err != nil {
		return err
	}

	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	data := map[string]any{}
	if strings.TrimSpace(c.Name) != "" {
		data["name"] = c.Name
	}
	if len(c.Scopes) > 0 {
		data["scopes"] = c.Scopes
	}
	if len(c.Assignments) > 0 {
		inlineBody, err := parseInlineAssignments(c.Assignments)
		if err != nil {
			return err
		}
		data, err = mergeJSONBodies(inlineBody, data)
		if err != nil {
			return fmt.Errorf("merge inline assignments: %w", err)
		}
	}

	name, ok := data["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return fmt.Errorf("token name is required; use --name or name=<value>")
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
