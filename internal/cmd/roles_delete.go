package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// RolesDeleteCmd deletes a role.
type RolesDeleteCmd struct {
	Role string `arg:"" help:"Role ID"`
}

// Run executes the delete command.
func (c *RolesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete role"); err != nil {
		return err
	}

	if err := requireForce(flags, "role "+c.Role); err != nil {
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

	path := "/roles/" + url.PathEscape(c.Role)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete role: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("role deleted"), []any{c.Role, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted role: %s\n", c.Role)
		return err
	})
}
