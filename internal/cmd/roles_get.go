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
	Role string `required:"" help:"Role ID"`
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

	return output.Detail(ctx, role, []any{role.ID, role.Name}, []output.Field{
		output.FAlways("ID", role.ID),
		output.FAlways("Name", role.Name),
		output.F("Description", role.Description),
		output.FAlways("Customers", len(role.Customers)),
		output.FAlways("Children", len(role.Children)),
		output.FAlways("Parents", len(role.Parents)),
	})
}
