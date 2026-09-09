package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// RolesUpdateCmd updates a role.
//
// Array fields (customers, children, parents) replace the whole relation.
// Server-side __op envelopes (AddReference/RemoveReference/Batch, plus
// AddRelation/RemoveRelation aliases) are sent verbatim.
type RolesUpdateCmd struct {
	Role        string   `required:"" help:"Role ID"`
	File        string   `help:"Read role JSON from file (use - for stdin)"`
	DryRun      bool     `name:"dry-run" help:"Print the request body and relation diff without writing"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=VIP)"`
}

// Run executes the update command.
func (c *RolesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "update role"); err != nil {
			return err
		}
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

	current, err := getRole(ctx, client, c.Role)
	if err != nil {
		return err
	}

	diffs, err := roleRelationDiffs(current, body)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}

	if humanOutput(ctx) {
		if err := printRoleRelationDiffs(ctx, diffs); err != nil {
			return err
		}
	}

	if err := requireRelationShrinks(flags, c.Role, diffs); err != nil {
		return err
	}

	if c.DryRun {
		return printRoleUpdateDryRun(ctx, body, diffs)
	}

	var role api.Role
	if err := client.Put(ctx, rolePath(c.Role), body, &role); err != nil {
		return fmt.Errorf("update role: %w", err)
	}

	verified, err := getRole(ctx, client, c.Role)
	if err != nil {
		return fmt.Errorf("verify role: %w", err)
	}

	if humanOutput(ctx) {
		if err := printRoleMemberCounts(ctx, verified); err != nil {
			return err
		}
	}

	return output.Print(ctx, verified, []any{verified.ID, verified.Name}, func() error {
		_, err := output.Fprintf(ctx, "Updated role: %s (%s)\n", verified.Name, verified.ID)
		return err
	})
}

func printRoleUpdateDryRun(ctx context.Context, body map[string]any, diffs []roleRelationDiff) error {
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, map[string]any{
			"dry_run":   true,
			"body":      body,
			"relations": diffs,
		})
	}

	encoded, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dry-run body: %w", err)
	}
	_, err = output.Fprintf(ctx, "%s\n", encoded)
	return err
}
