package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// AccountsListCmd lists accounts.
type AccountsListCmd struct {
	QueryFlags `embed:""`
	All        bool `help:"Fetch all pages"`
	Page       int  `help:"Page number" default:"1"`
	PerPage    int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *AccountsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	opts, err := listRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}

	var documents []api.Document[api.Account]
	if c.All {
		documents, err = api.List[api.Document[api.Account]](ctx, client, "/accounts", opts...)
		if err != nil {
			return fmt.Errorf("list accounts: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Document[api.Account]](ctx, client, "/accounts", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list accounts: %w", err)
		}
		documents = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, documents)
	}
	accounts := api.DocumentValues(documents)

	plainFields := []string{"id", "name", "plan", "owner"}
	tableFields := []string{"id", "name", "plan", "site_count", "users_count", "owner"}
	tableHeaders := []string{"ID", "NAME", "PLAN", "SITES", "USERS", "OWNER"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, accounts, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	return output.WriteTable(ctx, accounts, fields, headers)
}
