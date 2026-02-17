package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// RolesListCmd lists roles.
type RolesListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *RolesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list roles: %w", err)
	}

	var roles []api.Role
	if c.All {
		roles, err = api.List[api.Role](ctx, client, "/roles", opts...)
		if err != nil {
			return fmt.Errorf("list roles: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Role](ctx, client, "/roles", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list roles: %w", err)
		}
		roles = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, roles)
	}

	plainFields := []string{"id", "name", "customers"}
	tableFields := []string{"id", "name", "description"}
	tableHeaders := []string{"ID", "NAME", "DESCRIPTION"}

	if mode.Plain {
		if len(listRequestedFields(flags)) > 0 {
			return output.PlainFromSlice(ctx, roles, listOutputFields(flags, plainFields))
		}

		for _, role := range roles {
			if err := output.Plain(ctx, role.ID, role.Name, len(role.Customers)); err != nil {
				return err
			}
		}
		return nil
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, roles, fields, headers)
}
