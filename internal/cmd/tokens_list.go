package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// TokensListCmd lists API tokens.
type TokensListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *TokensListCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list tokens: %w", err)
	}

	var tokens []api.Token

	if c.All {
		tokens, err = api.List[api.Token](ctx, client, "/tokens", opts...)
		if err != nil {
			return fmt.Errorf("list tokens: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Token](ctx, client, "/tokens", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list tokens: %w", err)
		}
		tokens = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, tokens)
	}

	plainFields := []string{"id", "name"}
	tableFields := []string{"id", "name"}
	tableHeaders := []string{"ID", "NAME"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, tokens, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, tokens, fields, headers)
}
