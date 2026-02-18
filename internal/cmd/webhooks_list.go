package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// WebhooksListCmd lists webhooks.
type WebhooksListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *WebhooksListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
		return fmt.Errorf("list webhooks: %w", err)
	}

	var webhooks []api.Webhook
	var meta listFooterMeta

	if c.All {
		webhooks, err = api.List[api.Webhook](ctx, client, "/webhooks", opts...)
		if err != nil {
			return fmt.Errorf("list webhooks: %w", err)
		}
		meta = allListFooterMeta(len(webhooks))
	} else {
		paged, err := api.ListPage[api.Webhook](ctx, client, "/webhooks", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list webhooks: %w", err)
		}
		webhooks = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(webhooks))
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, webhooks)
	}

	plainFields := []string{"id", "url", "active"}
	tableFields := []string{"id", "url", "active"}
	tableHeaders := []string{"ID", "URL", "ACTIVE"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, webhooks, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, webhooks, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "webhooks", meta)
}
