package cmd

import (
	"context"
	"fmt"
	"slices"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// RolesCustomersCmd edits the customers relation on a role.
type RolesCustomersCmd struct {
	Add    RolesCustomersAddCmd    `cmd:"" help:"Add customers to a role"`
	Remove RolesCustomersRemoveCmd `cmd:"" help:"Remove customers from a role"`
	Set    RolesCustomersSetCmd    `cmd:"" help:"Replace a role's customer list"`
}

// RolesCustomersAddCmd adds customers to a role.
type RolesCustomersAddCmd struct {
	Role      string   `required:"" help:"Role ID"`
	Customers []string `required:"" name:"customer" help:"Customer ID to add, repeatable"`
}

func (c *RolesCustomersAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	return mutateRoleCustomers(ctx, flags, c.Role, func(current []string) []string {
		return uniqueStrings(append(slices.Clone(current), c.Customers...))
	})
}

// RolesCustomersRemoveCmd removes customers from a role.
type RolesCustomersRemoveCmd struct {
	Role      string   `required:"" help:"Role ID"`
	Customers []string `required:"" name:"customer" help:"Customer ID to remove, repeatable"`
}

func (c *RolesCustomersRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	return mutateRoleCustomers(ctx, flags, c.Role, func(current []string) []string {
		return uniqueStrings(subtractIDs(current, c.Customers))
	})
}

// RolesCustomersSetCmd replaces the customers on a role.
type RolesCustomersSetCmd struct {
	Role      string   `required:"" help:"Role ID"`
	Customers []string `required:"" name:"customer" help:"Customer ID that should remain, repeatable"`
}

func (c *RolesCustomersSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return mutateRoleCustomers(ctx, flags, c.Role, func([]string) []string {
		return uniqueStrings(c.Customers)
	})
}

func mutateRoleCustomers(ctx context.Context, flags *RootFlags, roleID string, nextFn func([]string) []string) error {
	if err := requireWrite(flags, "update role customers"); err != nil {
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

	current, err := getRole(ctx, client, roleID)
	if err != nil {
		return err
	}

	before := slices.Clone(current.Customers)
	next := nextFn(before)
	if err := requireRelationShrinks(flags, roleID, []roleRelationDiff{{
		Field:     "customers",
		Before:    len(before),
		After:     len(next),
		Projected: true,
	}}); err != nil {
		return err
	}

	if humanOutput(ctx) {
		if _, err := output.Fprintf(ctx, "customers: %d → %d\n", len(before), len(next)); err != nil {
			return err
		}
	}

	var updated api.Role
	if err := client.Put(ctx, rolePath(roleID), map[string]any{"customers": next}, &updated); err != nil {
		return fmt.Errorf("update role customers: %w", err)
	}

	verified, err := getRole(ctx, client, roleID)
	if err != nil {
		return fmt.Errorf("verify role customers: %w", err)
	}
	actual := uniqueStrings(verified.Customers)
	if !slices.Equal(actual, next) {
		return fmt.Errorf("role customers verification failed: requested %v, got %v", next, actual)
	}

	return output.Print(ctx, verified, []any{verified.ID, verified.Name, len(verified.Customers)}, func() error {
		_, err := output.Fprintf(ctx, "Updated role customers: %s (%s)\n", verified.Name, verified.ID)
		return err
	})
}
