package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// RolesGetCmd gets a role by ID.
type RolesGetCmd struct {
	Role string `arg:"" help:"Role ID"`
}

// Run executes the get command.
func (c *RolesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var role api.Role
	path := "/roles/" + url.PathEscape(c.Role)
	if err := client.Get(ctx, path, &role); err != nil {
		return fmt.Errorf("get role: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, role)
	}

	if mode.Plain {
		return output.Plain(ctx, role.ID, role.Name)
	}

	fmt.Printf("ID:          %s\n", role.ID)
	fmt.Printf("Name:        %s\n", role.Name)
	if role.Description != "" {
		fmt.Printf("Description: %s\n", role.Description)
	}
	fmt.Printf("Customers:   %d\n", len(role.Customers))
	fmt.Printf("Children:    %d\n", len(role.Children))
	fmt.Printf("Parents:     %d\n", len(role.Parents))

	return nil
}
