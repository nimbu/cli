package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeTemplatesListCmd lists theme templates.
type ThemeTemplatesListCmd struct {
	QueryFlags `embed:""`
	Theme      string `arg:"" help:"Theme ID"`
	All        bool   `help:"Fetch all pages"`
	Page       int    `help:"Page number" default:"1"`
	PerPage    int    `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *ThemeTemplatesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/themes/" + url.PathEscape(c.Theme) + "/templates"
	opts, err := listRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list theme templates: %w", err)
	}

	var templates []api.ThemeResource

	if c.All {
		templates, err = api.List[api.ThemeResource](ctx, client, path, opts...)
		if err != nil {
			return fmt.Errorf("list theme templates: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.ThemeResource](ctx, client, path, c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list theme templates: %w", err)
		}
		templates = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, templates)
	}

	plainFields := []string{"id", "name", "path", "updated_at"}
	tableFields := []string{"id", "name", "permalink", "updated_at"}
	tableHeaders := []string{"ID", "NAME", "PERMALINK", "UPDATED"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, templates, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	return output.WriteTable(ctx, templates, fields, headers)
}
