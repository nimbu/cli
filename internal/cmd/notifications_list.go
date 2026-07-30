package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// NotificationsListCmd lists notifications.
type NotificationsListCmd struct {
	QueryFlags `embed:""`
	All        bool `help:"Fetch all pages"`
	Page       int  `help:"Page number" default:"1"`
	PerPage    int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *NotificationsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	opts, err := localizedContentListRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list notifications: %w", err)
	}

	var documents []api.Document[api.Notification]
	var meta listFooterMeta
	if c.All {
		documents, err = api.List[api.Document[api.Notification]](ctx, client, "/notifications", opts...)
		if err != nil {
			return fmt.Errorf("list notifications: %w", err)
		}
		meta = allListFooterMeta(len(documents))
	} else {
		paged, err := api.ListPage[api.Document[api.Notification]](ctx, client, "/notifications", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list notifications: %w", err)
		}
		documents = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(documents))
		meta.probeTotal(ctx, client, "/notifications/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, documents)
	}
	notifications, err := localizedDocumentMaps(documents, c.Locale)
	if err != nil {
		return fmt.Errorf("project notification locale: %w", err)
	}

	plainFields := []string{"id", "slug", "name", "subject"}
	tableFields := []string{"id", "slug", "name", "subject", "html_enabled"}
	tableHeaders := []string{"ID", "SLUG", "NAME", "SUBJECT", "HTML"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, notifications, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, notifications, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "notifications", meta)
}
