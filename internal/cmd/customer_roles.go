package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type CustomerRolesCmd struct {
	List   CustomerRolesListCmd   `cmd:"" help:"List directly assigned customer roles"`
	Set    CustomerRolesSetCmd    `cmd:"" help:"Replace directly assigned customer roles"`
	Add    CustomerRolesAddCmd    `cmd:"" help:"Add directly assigned customer roles"`
	Remove CustomerRolesRemoveCmd `cmd:"" help:"Remove directly assigned customer roles"`
}

type CustomerRolesListCmd struct {
	Customer string `required:"" help:"Customer ID, slug, or email"`
}

func (c *CustomerRolesListCmd) Run(ctx context.Context) error {
	_, memberships, err := getCustomerRoleMemberships(ctx, c.Customer)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(memberships))
	for _, membership := range memberships {
		names = append(names, membership.Name)
	}
	slices.Sort(names)
	return output.JSON(ctx, names)
}

type CustomerRolesSetCmd struct {
	Customer string   `required:"" help:"Customer ID, slug, or email"`
	Roles    []string `required:"" name:"role" help:"Role ID or name, repeatable"`
}

func (c *CustomerRolesSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return setCustomerRoles(ctx, flags, c.Customer, c.Roles)
}

type CustomerRolesAddCmd struct {
	Customer string   `required:"" help:"Customer ID, slug, or email"`
	Roles    []string `required:"" name:"role" help:"Role ID or name, repeatable"`
}

func (c *CustomerRolesAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "add customer roles"); err != nil {
		return err
	}
	roles, current, err := getCustomerRoleMemberships(ctx, c.Customer)
	if err != nil {
		return err
	}
	requested, err := resolveCustomerRoleIDs(c.Roles, roles)
	if err != nil {
		return err
	}
	return writeCustomerRoles(ctx, c.Customer, uniqueStrings(append(membershipIDs(current), requested...)))
}

type CustomerRolesRemoveCmd struct {
	Customer string   `required:"" help:"Customer ID, slug, or email"`
	Roles    []string `required:"" name:"role" help:"Role ID or name, repeatable"`
}

func (c *CustomerRolesRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "remove customer roles"); err != nil {
		return err
	}
	roles, current, err := getCustomerRoleMemberships(ctx, c.Customer)
	if err != nil {
		return err
	}
	remove, err := resolveCustomerRoleIDs(c.Roles, roles)
	if err != nil {
		return err
	}
	next := make([]string, 0, len(current))
	for _, role := range current {
		if !slices.Contains(remove, role.ID) {
			next = append(next, role.ID)
		}
	}
	return writeCustomerRoles(ctx, c.Customer, next)
}

type customerRoleMembership struct {
	ID   string
	Name string
}

func getCustomerRoleMemberships(ctx context.Context, customer string) ([]api.Role, []customerRoleMembership, error) {
	client, err := auditedClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	var payload struct {
		ID string `json:"id"`
	}
	path := "/customers/" + url.PathEscape(customer)
	if err := client.Get(ctx, path, &payload); err != nil {
		return nil, nil, fmt.Errorf("get customer: %w", err)
	}
	if payload.ID == "" {
		return nil, nil, fmt.Errorf("get customer roles: customer response has no id")
	}
	roles, err := api.List[api.Role](ctx, client, "/roles")
	if err != nil {
		return nil, nil, fmt.Errorf("list roles: %w", err)
	}
	memberships := make([]customerRoleMembership, 0)
	for _, role := range roles {
		if containsCustomerReference(role.Customers, payload.ID) {
			memberships = append(memberships, customerRoleMembership{ID: role.ID, Name: role.Name})
		}
	}
	return roles, memberships, nil
}

func containsCustomerReference(references []string, customerID string) bool {
	for _, reference := range references {
		trimmed := strings.TrimRight(reference, "/")
		if trimmed == customerID || strings.HasSuffix(trimmed, "/"+customerID) {
			return true
		}
	}
	return false
}

func resolveCustomerRoleIDs(requested []string, roles []api.Role) ([]string, error) {
	ids := make([]string, 0, len(requested))
	for _, reference := range uniqueStrings(requested) {
		found := ""
		for _, role := range roles {
			if reference == role.ID || reference == role.Name {
				found = role.ID
				break
			}
		}
		if found == "" {
			return nil, fmt.Errorf("unknown customer role %q", reference)
		}
		ids = append(ids, found)
	}
	return uniqueStrings(ids), nil
}

func setCustomerRoles(ctx context.Context, flags *RootFlags, customer string, roles []string) error {
	if err := requireWrite(flags, "update customer roles"); err != nil {
		return err
	}
	available, _, err := getCustomerRoleMemberships(ctx, customer)
	if err != nil {
		return err
	}
	roleIDs, err := resolveCustomerRoleIDs(roles, available)
	if err != nil {
		return err
	}
	return writeCustomerRoles(ctx, customer, roleIDs)
}

func writeCustomerRoles(ctx context.Context, customer string, roleIDs []string) error {
	client, err := auditedClient(ctx)
	if err != nil {
		return err
	}
	path := "/customers/" + url.PathEscape(customer)
	var result json.RawMessage
	roleIDs = uniqueStrings(roleIDs)
	if err := client.Put(ctx, path, map[string]any{"roles": roleIDs}, &result); err != nil {
		return fmt.Errorf("update customer roles: %w", err)
	}
	_, actual, err := getCustomerRoleMemberships(ctx, customer)
	if err != nil {
		return fmt.Errorf("verify customer roles: %w", err)
	}
	actualIDs := membershipIDs(actual)
	if !slices.Equal(actualIDs, roleIDs) {
		return fmt.Errorf("customer roles verification failed: requested %v, got %v", roleIDs, actualIDs)
	}
	return output.JSON(ctx, result)
}

func membershipIDs(memberships []customerRoleMembership) []string {
	ids := make([]string, 0, len(memberships))
	for _, membership := range memberships {
		ids = append(ids, membership.ID)
	}
	return uniqueStrings(ids)
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return result
}
