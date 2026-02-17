package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// AccountsListCmd lists accounts.
type AccountsListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *AccountsListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
		return fmt.Errorf("list accounts: %w", err)
	}

	var accounts []api.Account
	if c.All {
		accounts, err = api.List[api.Account](ctx, client, "/accounts", opts...)
		if err != nil {
			return fmt.Errorf("list accounts: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Account](ctx, client, "/accounts", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list accounts: %w", err)
		}
		accounts = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, accounts)
	}

	plainFields := []string{"id", "name", "plan", "owner"}
	tableFields := []string{"id", "name", "plan", "site_count", "users_count", "owner"}
	tableHeaders := []string{"ID", "NAME", "PLAN", "SITES", "USERS", "OWNER"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, accounts, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, accounts, fields, headers)
}
