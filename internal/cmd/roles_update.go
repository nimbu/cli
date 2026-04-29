package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// RolesUpdateCmd updates a role.
type RolesUpdateCmd struct {
	Role        string   `required:"" help:"Role ID"`
	File        string   `help:"Read role JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=VIP)"`
}

// Run executes the update command.
func (c *RolesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update role"); err != nil {
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
	path := "/roles/" + url.PathEscape(c.Role)
	if err := client.Put(ctx, path, body, &role); err != nil {
		return fmt.Errorf("update role: %w", err)
	}

	return output.Print(ctx, role, []any{role.ID, role.Name}, func() error {
		_, err := output.Fprintf(ctx, "Updated role: %s (%s)\n", role.Name, role.ID)
		return err
	})
}
