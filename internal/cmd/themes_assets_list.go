package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeAssetsListCmd lists theme assets.
type ThemeAssetsListCmd struct {
	QueryFlags `embed:""`
	Theme      string `arg:"" help:"Theme ID"`
	All        bool   `help:"Fetch all pages"`
	Page       int    `help:"Page number" default:"1"`
	PerPage    int    `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *ThemeAssetsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/themes/" + url.PathEscape(c.Theme) + "/assets"
	opts, err := listRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list theme assets: %w", err)
	}

	var assets []api.ThemeResource

	if c.All {
		assets, err = api.List[api.ThemeResource](ctx, client, path, opts...)
		if err != nil {
			return fmt.Errorf("list theme assets: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.ThemeResource](ctx, client, path, c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list theme assets: %w", err)
		}
		assets = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, assets)
	}

	plainFields := []string{"id", "path", "name", "updated_at"}
	tableFields := []string{"id", "name", "path", "folder", "public_url", "updated_at"}
	tableHeaders := []string{"ID", "NAME", "PATH", "FOLDER", "PUBLIC URL", "UPDATED"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, assets, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	return output.WriteTable(ctx, assets, fields, headers)
}
