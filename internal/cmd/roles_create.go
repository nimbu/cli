package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// RolesCreateCmd creates a role.
type RolesCreateCmd struct {
	File        string   `help:"Read role JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=VIP)"`
}

// Run executes the create command.
func (c *RolesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create role"); err != nil {
		return err
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	body, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		return err
	}

	var role api.Role
	if err := client.Post(ctx, "/roles", body, &role); err != nil {
		return fmt.Errorf("create role: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, role)
	}

	if mode.Plain {
		return output.Plain(ctx, role.ID, role.Name)
	}

	fmt.Printf("Created role: %s (%s)\n", role.Name, role.ID)
	return nil
}
